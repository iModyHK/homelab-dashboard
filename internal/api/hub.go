package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/iModyHK/homelab-dashboard/internal/collector"
)

type hub struct {
	bus     *collector.Bus
	log     *slog.Logger
	mu      sync.Mutex
	conns   map[*websocket.Conn]struct{}
	origins []string
}

func newHub(bus *collector.Bus, logger *slog.Logger) *hub {
	return &hub{bus: bus, log: logger, conns: map[*websocket.Conn]struct{}{}}
}

func (h *hub) setOrigins(origins []string) {
	h.origins = origins
}

func (h *hub) clients() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.conns)
}

func (h *hub) run(stop <-chan struct{}) {
	<-stop
	h.mu.Lock()
	for c := range h.conns {
		c.Close(websocket.StatusGoingAway, "server shutting down")
	}
	h.mu.Unlock()
}

func (h *hub) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns:  h.origins,
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		h.log.Debug("websocket accept", "error", err)
		return
	}
	h.mu.Lock()
	h.conns[conn] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.conns, conn)
		h.mu.Unlock()
		conn.CloseNow()
	}()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	messages, unsubscribe := h.bus.Subscribe(128)
	defer unsubscribe()

	go func() {
		defer cancel()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}()

	if err := h.write(ctx, conn, collector.Message{Type: "hello", Data: map[string]int64{"ts": time.Now().Unix()}}); err != nil {
		return
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				return
			}
		case msg := <-messages:
			if err := h.write(ctx, conn, msg); err != nil {
				return
			}
		}
	}
}

func (h *hub) write(ctx context.Context, conn *websocket.Conn, msg collector.Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, payload)
}
