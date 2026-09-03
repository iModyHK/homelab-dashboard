package alerts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/collector"
	"github.com/iModyHK/homelab-dashboard/internal/config"
	"github.com/iModyHK/homelab-dashboard/internal/store"
)

type Notifier interface {
	Send(ctx context.Context, text string) error
	Name() string
}

type condition struct {
	rule       string
	target     string
	targetName string
	severity   string
	message    string
	holdFor    time.Duration
}

type track struct {
	pendingSince   time.Time
	clearSince     time.Time
	alertID        int64
	lastTransition time.Time
	message        string
	targetName     string
	severity       string
}

type notification struct {
	text    string
	alertID int64
}

type Engine struct {
	thresholds config.AlertThresholds
	dbMax      int64
	store      *store.Store
	bus        *collector.Bus
	notifier   Notifier
	log        *slog.Logger
	tz         *time.Location

	mu     sync.Mutex
	tracks map[string]*track
	queue  chan notification
}

func NewEngine(cfg *config.Config, st *store.Store, bus *collector.Bus, notifier Notifier, logger *slog.Logger) *Engine {
	return &Engine{
		thresholds: cfg.Alerts,
		dbMax:      cfg.DBMaxBytes,
		store:      st,
		bus:        bus,
		notifier:   notifier,
		log:        logger,
		tz:         cfg.Timezone,
		tracks:     map[string]*track{},
		queue:      make(chan notification, 256),
	}
}

func (e *Engine) Restore(ctx context.Context) error {
	firing, err := e.store.FiringAlerts(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, a := range firing {
		e.tracks[key(a.Rule, a.Target)] = &track{
			alertID: a.ID, lastTransition: time.Unix(a.FiredAt, 0), message: a.Message, targetName: a.TargetName, severity: a.Severity,
		}
	}
	return nil
}

func (e *Engine) Run(ctx context.Context) {
	if e.notifier == nil {
		<-ctx.Done()
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-e.queue:
			e.deliver(ctx, n)
		}
	}
}

func (e *Engine) deliver(ctx context.Context, n notification) {
	delays := []time.Duration{2 * time.Second, 8 * time.Second, 32 * time.Second, 2 * time.Minute, 5 * time.Minute}
	for attempt := 0; attempt <= len(delays); attempt++ {
		sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := e.notifier.Send(sendCtx, n.text)
		cancel()
		if err == nil {
			if n.alertID > 0 {
				if err := e.store.MarkAlertNotified(ctx, n.alertID, time.Now()); err != nil {
					e.log.Warn("mark notified", "error", err)
				}
			}
			return
		}
		e.log.Warn("notification failed", "channel", e.notifier.Name(), "attempt", attempt+1, "error", err)
		if attempt == len(delays) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delays[attempt]):
		}
	}
}

var ErrNoNotifier = errors.New("no alert channel configured")

func (e *Engine) SendTest(ctx context.Context) error {
	if e.notifier == nil {
		return ErrNoNotifier
	}
	text := fmt.Sprintf("🔔 <b>test</b> · Homelab Dashboard\nAlert delivery works.\n<i>%s</i>", time.Now().In(e.tz).Format("Mon 02 Jan 15:04"))
	return e.notifier.Send(ctx, text)
}

func (e *Engine) enqueue(n notification) {
	if e.notifier == nil {
		return
	}
	select {
	case e.queue <- n:
	default:
		e.log.Warn("notification queue full, dropping", "text", n.text)
	}
}

func (e *Engine) Evaluate(ctx context.Context, snap collector.Snapshot, restarts map[string]int) {
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()

	active := map[string]condition{}
	for _, cond := range e.conditions(snap, restarts) {
		active[key(cond.rule, cond.target)] = cond
	}

	for k, cond := range active {
		tr, ok := e.tracks[k]
		if !ok {
			tr = &track{}
			e.tracks[k] = tr
		}
		tr.clearSince = time.Time{}
		tr.targetName = cond.targetName
		tr.severity = cond.severity
		if tr.alertID > 0 {
			tr.message = cond.message
			continue
		}
		if tr.pendingSince.IsZero() {
			tr.pendingSince = now
		}
		if now.Sub(tr.pendingSince) < cond.holdFor {
			continue
		}
		if !tr.lastTransition.IsZero() && now.Sub(tr.lastTransition) < e.thresholds.Debounce {
			continue
		}
		e.fire(ctx, now, cond, tr)
	}

	for k, tr := range e.tracks {
		if _, stillActive := active[k]; stillActive {
			continue
		}
		tr.pendingSince = time.Time{}
		if tr.alertID == 0 {
			delete(e.tracks, k)
			continue
		}
		if tr.clearSince.IsZero() {
			tr.clearSince = now
		}
		if now.Sub(tr.clearSince) < e.thresholds.Debounce {
			continue
		}
		e.resolve(ctx, now, k, tr)
	}
}

func (e *Engine) fire(ctx context.Context, now time.Time, cond condition, tr *track) {
	rec := store.AlertRecord{
		Rule: cond.rule, Target: cond.target, TargetName: cond.targetName, Severity: cond.severity,
		Message: cond.message, State: "firing", FiredAt: now.Unix(),
	}
	id, err := e.store.OpenAlert(ctx, rec)
	if err != nil {
		e.log.Error("open alert", "error", err)
		return
	}
	rec.ID = id
	tr.alertID = id
	tr.lastTransition = now
	tr.message = cond.message
	tr.pendingSince = time.Time{}
	e.log.Warn("alert firing", "rule", cond.rule, "target", cond.targetName, "message", cond.message)
	e.bus.Publish("alert", rec)
	e.enqueue(notification{alertID: id, text: e.format("firing", cond.severity, cond.rule, cond.targetName, cond.message, now)})
}

func (e *Engine) resolve(ctx context.Context, now time.Time, k string, tr *track) {
	if err := e.store.ResolveAlert(ctx, tr.alertID, now); err != nil {
		e.log.Error("resolve alert", "error", err)
		return
	}
	rule, _ := splitKey(k)
	e.log.Info("alert resolved", "rule", rule, "target", tr.targetName)
	e.bus.Publish("alert", store.AlertRecord{
		ID: tr.alertID, Rule: rule, TargetName: tr.targetName, Severity: tr.severity, Message: tr.message,
		State: "resolved", ResolvedAt: now.Unix(),
	})
	e.enqueue(notification{text: e.format("resolved", tr.severity, rule, tr.targetName, tr.message, now)})
	delete(e.tracks, k)
}

func (e *Engine) isFiring(rule, target string) bool {
	tr, ok := e.tracks[key(rule, target)]
	return ok && tr.alertID > 0
}

func (e *Engine) format(state, severity, rule, target, message string, at time.Time) string {
	icon := "🔴"
	if state == "resolved" {
		icon = "🟢"
	} else if severity == "warning" {
		icon = "🟠"
	}
	return fmt.Sprintf("%s <b>%s</b> · %s\n<b>%s</b>\n%s\n<i>%s</i>",
		icon, state, ruleTitle(rule), escape(target), escape(message), at.In(e.tz).Format("Mon 02 Jan 15:04"))
}

func ruleTitle(rule string) string {
	titles := map[string]string{
		"container_down":   "Container down",
		"restart_loop":     "Restart loop",
		"health_failing":   "Health check failing",
		"cpu_high":         "CPU high",
		"memory_high":      "Memory high",
		"disk_high":        "Disk almost full",
		"smart_failing":    "SMART failure",
		"disk_temp_high":   "Drive temperature high",
		"raid_degraded":    "RAID degraded",
		"host_unreachable": "Host unreachable",
		"db_size":          "Dashboard database large",
		"host_cpu_high":    "Host CPU high",
		"host_memory_high": "Host memory high",
	}
	if t, ok := titles[rule]; ok {
		return t
	}
	return rule
}

func key(rule, target string) string {
	return rule + "|" + target
}

func splitKey(k string) (string, string) {
	for i := 0; i < len(k); i++ {
		if k[i] == '|' {
			return k[:i], k[i+1:]
		}
	}
	return k, ""
}

func escape(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			out = append(out, "&lt;"...)
		case '>':
			out = append(out, "&gt;"...)
		case '&':
			out = append(out, "&amp;"...)
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
