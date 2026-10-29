// Package scan runs one pass over active trips and schedules passes on a timer.
// The per-mode search algorithms (design_doc.md "Scan Algorithm") plug in here once
// a CashSource/AwardSource exists; with none registered every trip records a scan error
// so the Settings page shows exactly why nothing is being found.
package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/arjungandhi/flights/pkg/db"
	"github.com/arjungandhi/flights/pkg/model"
	"github.com/arjungandhi/flights/pkg/source"
)

var ErrNoSources = errors.New("no cash or award source configured")

type Runner struct {
	DB     *db.DB
	Cash   source.CashSource  // nil until a source is configured
	Award  source.AwardSource // nil until a source is configured
	Notify Notifier

	mu      sync.Mutex
	running bool
}

// Message is one alert in `messages send` stdin format.
type Message struct {
	RoomID string `json:"room_id"`
	Text   string `json:"text"`
}

type Notifier interface {
	Send(msgs []Message) error
}

// Run scans every active trip (or just tripID when non-empty). Returns ErrNoSources
// after recording one scan row per trip when nothing is configured.
func (r *Runner) Run(ctx context.Context, tripID string) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return errors.New("scan already in progress")
	}
	r.running = true
	r.mu.Unlock()
	defer func() { r.mu.Lock(); r.running = false; r.mu.Unlock() }()

	trips, err := r.DB.Trips()
	if err != nil {
		return err
	}
	var errs []string
	for _, t := range trips {
		if t.Status != model.StatusActive || (tripID != "" && t.ID != tripID) {
			continue
		}
		if err := r.scanTrip(ctx, t); err != nil {
			errs = append(errs, t.ID+": "+err.Error())
		}
	}
	if r.Cash == nil && r.Award == nil {
		return ErrNoSources
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d trip(s) failed: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

func (r *Runner) scanTrip(ctx context.Context, t model.Trip) error {
	s := model.Scan{TripID: t.ID, StartedAt: time.Now()}
	switch {
	case r.Cash == nil && r.Award == nil:
		s.Source, s.Error = "none", ErrNoSources.Error()
	default:
		s.Source = sourceNames(r)
		s.Error = "scan algorithm not implemented for mode " + string(t.Mode)
	}
	s.FinishedAt = time.Now()
	if _, err := r.DB.AddScan(s); err != nil {
		return err
	}
	if s.Error != "" {
		return errors.New(s.Error)
	}
	return nil
}

func sourceNames(r *Runner) string {
	var names []string
	if r.Cash != nil {
		names = append(names, r.Cash.Name())
	}
	if r.Award != nil {
		names = append(names, r.Award.Name())
	}
	return strings.Join(names, "+")
}

// Loop runs a scan every interval until ctx is done. The first run is immediate.
func (r *Runner) Loop(ctx context.Context, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		if err := r.Run(ctx, ""); err != nil {
			log.Printf("scan: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// ExecNotifier pipes JSON lines to a shell command's stdin, e.g. `messages send`.
type ExecNotifier struct {
	Command string
}

func (n ExecNotifier) Send(msgs []Message) error {
	if n.Command == "" || len(msgs) == 0 {
		return nil
	}
	cmd := exec.Command("sh", "-c", n.Command)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	enc := json.NewEncoder(stdin)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			stdin.Close()
			return err
		}
	}
	stdin.Close()
	return cmd.Wait()
}

// WriterNotifier writes JSON lines to w; used when no --notify command is set.
type WriterNotifier struct{ W io.Writer }

func (n WriterNotifier) Send(msgs []Message) error {
	enc := json.NewEncoder(n.W)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			return err
		}
	}
	return nil
}
