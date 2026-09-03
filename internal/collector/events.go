package collector

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/docker"
	"github.com/iModyHK/homelab-dashboard/internal/store"
)

func (c *Collector) eventsLoop(ctx context.Context) {
	backoff := time.Second
	since := time.Now()
	for {
		if ctx.Err() != nil {
			return
		}
		start := time.Now()
		err := c.docker.Events(ctx, since, func(ev docker.Event) {
			since = time.Unix(ev.Time, 0)
			c.handleEvent(ctx, ev)
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.log.Warn("event stream", "error", err, "retry_in", backoff)
		}
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

var interestingActions = map[string]bool{
	"start": true, "stop": true, "die": true, "kill": true, "restart": true, "oom": true,
	"pause": true, "unpause": true, "create": true, "destroy": true, "rename": true, "update": true,
}

func (c *Collector) handleEvent(ctx context.Context, ev docker.Event) {
	if ev.Type != "container" {
		return
	}
	action := ev.Action
	detail := ""
	if strings.HasPrefix(action, "health_status") {
		_, status, _ := strings.Cut(action, ": ")
		action = "health_status"
		detail = status
	} else if strings.HasPrefix(action, "exec_") {
		return
	} else if !interestingActions[action] {
		return
	}

	name := ev.Actor.Attributes["name"]
	project := ev.Actor.Attributes[docker.LabelComposeProject]
	if c.excluded(name, project) {
		return
	}
	id := ev.Actor.ID
	if action == "die" {
		if code, ok := ev.Actor.Attributes["exitCode"]; ok {
			detail = "exit " + code
		}
	}
	if action == "oom" {
		detail = "out of memory"
	}
	ts := ev.Time
	if ev.TimeNano > 0 {
		ts = ev.TimeNano / 1e9
	}

	record := store.EventRecord{TS: ts, ContainerID: id, ContainerName: name, Type: action, Detail: detail}
	eventID, err := c.store.InsertEvent(ctx, record)
	if err != nil {
		c.log.Warn("store event", "error", err)
	}
	record.ID = eventID

	switch action {
	case "die":
		code, _ := strconv.Atoi(ev.Actor.Attributes["exitCode"])
		if code != 0 {
			c.recordErrorLine(ctx, store.LogErrorRecord{
				TS: ts, ContainerID: id, ContainerName: name, Kind: "exit", Stream: "event",
				Line: fmt.Sprintf("container exited with code %d", code),
			})
		}
		c.ResetCounters(id)
	case "oom":
		c.recordErrorLine(ctx, store.LogErrorRecord{
			TS: ts, ContainerID: id, ContainerName: name, Kind: "oom", Stream: "event", Line: "container killed by the OOM killer",
		})
	case "health_status":
		if detail == "unhealthy" {
			c.recordErrorLine(ctx, store.LogErrorRecord{
				TS: ts, ContainerID: id, ContainerName: name, Kind: "health", Stream: "event", Line: "health check failing",
			})
		}
	case "start", "restart":
		c.ResetCounters(id)
	case "destroy":
		c.state.mu.Lock()
		delete(c.state.containers, id)
		c.state.mu.Unlock()
		c.publishContainers()
		c.bus.Publish("event", record)
		return
	}

	c.state.mu.RLock()
	_, known := c.state.containers[id]
	c.state.mu.RUnlock()
	if !known && action == "create" {
		c.refreshInventory(ctx)
	} else if known || action == "start" {
		if !known {
			c.refreshInventory(ctx)
		} else {
			c.inspect(ctx, id)
			c.persistInventory(ctx, time.Now())
			c.publishContainers()
		}
	}
	c.bus.Publish("event", record)
	c.evaluate(ctx)
}

func (c *Collector) recordErrorLine(ctx context.Context, r store.LogErrorRecord) {
	if _, err := c.store.InsertLogErrors(ctx, []store.LogErrorRecord{r}); err != nil {
		c.log.Warn("store error line", "error", err)
		return
	}
	c.bus.Publish("error", r)
}
