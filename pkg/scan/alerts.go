package scan

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/arjungandhi/flights/pkg/db"
	"github.com/arjungandhi/flights/pkg/geo"
	"github.com/arjungandhi/flights/pkg/model"
	"github.com/arjungandhi/flights/pkg/pricing"
)

// Message is one alert in `messages send` stdin format.
type Message struct {
	RoomID string `json:"room_id"`
	Text   string `json:"text"`
}

type Notifier interface {
	Send(msgs []Message) error
}

const (
	realertDrop = 0.90 // re-alert the same dates only when ≥10% cheaper than last time
	newLowDrop  = 0.90 // "new low" fires when ≥10% below the lowest price ever alerted
	batchMax    = 5    // flexible/explore: one message per scan with at most this many candidates
)

// alert applies the rules from design_doc.md to ranked candidates, records what fires, and sends.
//
//  1. under target: per-person effective ≤ target (dedupe per dates, re-alert on ≥10% drop).
//     Fixed trips send one message per candidate; flexible/explore send one batched message.
//  2. new low (fixed/flexible): best candidate ≥10% below anything alerted before, or first ever
//  3. award space opened: not applicable until an award source exists
//  4. explore without target: every traveler's fare sits at the provider's "low" level
func (r *Runner) alert(t model.Trip, cands []pricing.Candidate, s db.Settings) error {
	if len(cands) == 0 {
		return nil
	}
	minAlerted, hasAlerted, err := r.DB.MinAlerted(t.ID)
	if err != nil {
		return err
	}
	var fired []pricing.Candidate
	for i, c := range cands {
		pp := c.PerPerson()
		under := t.TargetUSD > 0 && pp <= t.TargetUSD
		newLow := i == 0 && t.Mode != model.ModeExplore && (!hasAlerted || pp <= minAlerted*newLowDrop)
		low := t.Mode == model.ModeExplore && t.TargetUSD == 0 && allLow(c)
		if !under && !newLow && !low {
			continue
		}
		prev, ok, err := r.DB.LastAlert(t.ID, c.Dest, c.OutDate, c.RetDate)
		if err != nil {
			return err
		}
		if ok && pp > prev.EffectiveUSD*realertDrop {
			continue
		}
		fired = append(fired, c)
		if t.Mode != model.ModeFixed && len(fired) >= batchMax {
			break
		}
	}
	if len(fired) == 0 {
		return nil
	}
	now := time.Now()
	for _, c := range fired {
		if err := r.DB.AddAlert(model.Alert{TripID: t.ID, Dest: c.Dest, OutDate: c.OutDate, RetDate: c.RetDate,
			Program: programs(c), EffectiveUSD: c.PerPerson(), SentAt: now}); err != nil {
			return err
		}
	}
	var msgs []Message
	if t.Mode != model.ModeFixed {
		msgs = append(msgs, Message{RoomID: s.NotifyRoom, Text: batchText(t, fired, s)})
	} else {
		for _, c := range fired {
			msgs = append(msgs, Message{RoomID: s.NotifyRoom, Text: candidateText(t, c, s)})
		}
	}
	if r.Notify == nil {
		return nil
	}
	return r.Notify.Send(msgs)
}

func allLow(c pricing.Candidate) bool {
	for _, p := range c.Picks {
		if p.UsesAward || p.Cash == nil || p.Cash.PriceLevel != 1 {
			return false
		}
	}
	return true
}

func programs(c pricing.Candidate) string {
	var ps []string
	for _, p := range c.Picks {
		if p.UsesAward && p.Award != nil {
			ps = append(ps, p.Award.Program)
		}
	}
	return strings.Join(ps, ",")
}

func candidateText(t model.Trip, c pricing.Candidate, s db.Settings) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✈ %s · %s %s · $%.0f/pp · $%.0f total", t.ID, where(c.Dest), dates(c), c.PerPerson(), c.Total)
	if t.TargetUSD > 0 {
		fmt.Fprintf(&b, " · target $%.0f", t.TargetUSD)
	}
	for _, p := range c.Picks {
		b.WriteString("\n  " + pickText(p, s))
	}
	if s.BaseURL != "" {
		b.WriteString("\n  " + s.BaseURL + "/trips/" + t.ID)
	}
	return b.String()
}

func batchText(t model.Trip, cs []pricing.Candidate, s db.Settings) string {
	var b strings.Builder
	if t.Mode == model.ModeExplore {
		fmt.Fprintf(&b, "✈ %s · %d cheap destination(s) for everyone", t.ID, len(cs))
	} else {
		fmt.Fprintf(&b, "✈ %s · %s · %d date option(s) under $%.0f/pp", t.ID, where(t.Dest), len(cs), t.TargetUSD)
	}
	for _, c := range cs {
		fmt.Fprintf(&b, "\n%s %s · $%.0f/pp · $%.0f total", where(c.Dest), dates(c), c.PerPerson(), c.Total)
		for _, p := range c.Picks {
			b.WriteString("\n  " + pickText(p, s))
		}
	}
	if s.BaseURL != "" {
		if t.Mode == model.ModeExplore {
			b.WriteString("\n" + s.BaseURL + "/explore")
		} else {
			b.WriteString("\n" + s.BaseURL + "/trips/" + t.ID)
		}
	}
	return b.String()
}

func pickText(p pricing.Pick, s db.Settings) string {
	if p.UsesAward && p.Award != nil {
		return fmt.Sprintf("%s: %s %dk %s + $%.0f (= $%.0f)", p.Traveler.Name, p.Award.Origin, p.Award.Miles/1000, p.Award.Program, p.Award.TaxesUSD, p.Effective)
	}
	o := p.Cash
	stops := "nonstop"
	if o.Stops == 1 {
		stops = "1 stop"
	} else if o.Stops > 1 {
		stops = fmt.Sprintf("%d stops", o.Stops)
	}
	txt := fmt.Sprintf("%s: %s $%.0f (%s %s)", p.Traveler.Name, o.Origin, o.PriceUSD, o.Airline, stops)
	if o.TypicalLow > 0 {
		txt += fmt.Sprintf(" · typical $%.0f–%.0f", o.TypicalLow, o.TypicalHi)
	}
	return txt
}

func where(code string) string {
	if city := geo.City(code); city != "" {
		return city + " (" + code + ")"
	}
	return code
}

func dates(c pricing.Candidate) string {
	o, _ := time.Parse(model.DateLayout, c.OutDate)
	if c.RetDate == "" {
		return o.Format("Jan 2")
	}
	r, _ := time.Parse(model.DateLayout, c.RetDate)
	return o.Format("Jan 2") + "–" + r.Format("Jan 2")
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
