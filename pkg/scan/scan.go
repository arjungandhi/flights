// Package scan runs one pass over active trips and schedules passes on a timer.
// See design_doc.md "Scan Algorithm" and "Alert Rules".
package scan

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/arjungandhi/flights/pkg/db"
	"github.com/arjungandhi/flights/pkg/model"
	"github.com/arjungandhi/flights/pkg/pricing"
	"github.com/arjungandhi/flights/pkg/source"
)

var ErrNoSources = errors.New("no cash source configured")

// Page budgets per trip per scan. Each Google page is ~2-3 MB, so these bound bandwidth.
const (
	coarseFixed    = 25
	coarseFlexible = 16
	refineTop      = 3
	refineDays     = 2
	maxPagesTrip   = 160
	maxFailures    = 5
	// Flexible trips are page-hungry; they rescan once a day unless forced.
	slowRescan = 20 * time.Hour
)

type Runner struct {
	DB     *db.DB
	Cash   source.CashSource
	Award  source.AwardSource
	Notify Notifier

	mu      sync.Mutex
	running bool
}

// Busy reports whether a pass is in progress.
func (r *Runner) Busy() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// Run scans every active trip, or just tripID when non-empty. force bypasses the daily
// throttle for flexible trips (the "Scan now" button).
func (r *Runner) Run(ctx context.Context, tripID string, force bool) error {
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
	last, err := r.DB.LastScan()
	if err != nil {
		return err
	}
	var errs []string
	for _, t := range trips {
		if t.Status != model.StatusActive || (tripID != "" && t.ID != tripID) {
			continue
		}
		if !force && t.Mode != model.ModeFixed {
			if ls, ok := last[t.ID]; ok && ls.Error == "" && time.Since(ls.StartedAt) < slowRescan {
				continue
			}
		}
		if err := r.scanTrip(ctx, t); err != nil {
			if errors.Is(err, ErrNoSources) {
				return err
			}
			errs = append(errs, t.ID+": "+err.Error())
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d trip(s) failed: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

func (r *Runner) scanTrip(ctx context.Context, t model.Trip) error {
	s := model.Scan{TripID: t.ID, StartedAt: time.Now()}
	if r.Cash == nil {
		s.Source, s.Error, s.FinishedAt = "none", ErrNoSources.Error(), time.Now()
		r.DB.AddScan(s)
		return ErrNoSources
	}
	s.Source = r.Cash.Name()
	settings, err := r.DB.Settings()
	if err != nil {
		return err
	}
	offers, calls, perr := r.price(ctx, t, settings)
	s.CallsUsed, s.FinishedAt = calls, time.Now()
	if perr != nil {
		s.Error = perr.Error()
	}
	scanID, err := r.DB.AddScan(s)
	if err != nil {
		return err
	}
	if len(offers) > 0 {
		for i := range offers {
			offers[i].ScanID = scanID
		}
		if err := r.DB.AddOffers(offers); err != nil {
			return err
		}
		if err := r.recordHistory(offers, s.StartedAt); err != nil {
			return err
		}
	}
	if perr != nil {
		return perr
	}
	return r.alert(t, pricing.Candidates(t, offers, settings), settings)
}

// recordHistory keeps one lowest-cash price per route per day.
func (r *Runner) recordHistory(offers []model.Offer, day time.Time) error {
	type route struct{ o, d string }
	low := map[route]float64{}
	for _, o := range offers {
		if o.Kind != model.KindCash {
			continue
		}
		k := route{o.Origin, o.Dest}
		if v, ok := low[k]; !ok || o.PriceUSD < v {
			low[k] = o.PriceUSD
		}
	}
	for k, v := range low {
		if err := r.DB.RecordPrice(k.o, k.d, day, v); err != nil {
			return err
		}
	}
	return nil
}

// priceRun carries the per-scan page budget and the set of (traveler, home, dest, pair) done.
type priceRun struct {
	r        *Runner
	t        model.Trip
	settings db.Settings
	calls    int
	failures int
	offers   []model.Offer
	done     map[string]bool
	lastErr  error
}

func (r *Runner) price(ctx context.Context, t model.Trip, settings db.Settings) ([]model.Offer, int, error) {
	pairs := Pairs(t, time.Now())
	if len(pairs) == 0 {
		return nil, 0, errors.New("no searchable dates (window in the past?)")
	}
	travelers := t.Unbooked()
	if len(travelers) == 0 {
		return nil, 0, errors.New("everyone is booked")
	}
	run := &priceRun{r: r, t: t, settings: settings, done: map[string]bool{}}

	coarse := coarseFixed
	if t.Mode == model.ModeFlexible {
		coarse = coarseFlexible
	}
	sample := Sample(pairs, coarse)
	if err := run.priceSet(ctx, travelers, t.Dest, sample); err != nil {
		return run.offers, run.calls, err
	}

	// Refine around what looks cheapest for the whole group.
	cands := pricing.Candidates(t, run.offers, settings)
	skip := map[Pair]bool{}
	for _, p := range sample {
		skip[p] = true
	}
	for i, c := range cands {
		if i >= refineTop {
			break
		}
		more := Neighbors(pairs, Pair{c.OutDate, c.RetDate}, refineDays, skip)
		for _, p := range more {
			skip[p] = true
		}
		if err := run.priceSet(ctx, travelers, t.Dest, more); err != nil {
			return run.offers, run.calls, err
		}
	}
	if run.lastErr != nil && len(run.offers) == 0 {
		return nil, run.calls, run.lastErr
	}
	return run.offers, run.calls, nil
}

// priceSet fetches every (traveler, home, pair) for one destination.
func (p *priceRun) priceSet(ctx context.Context, travelers []model.TripTraveler, dest string, pairs []Pair) error {
	for _, pair := range pairs {
		for _, tr := range travelers {
			for _, home := range tr.Homes {
				if home == dest {
					continue
				}
				key := fmt.Sprintf("%d|%s|%s|%s|%s", tr.ID, home, dest, pair.Out, pair.Ret)
				if p.done[key] {
					continue
				}
				if p.calls >= maxPagesTrip {
					return fmt.Errorf("page budget (%d) reached; results are partial", maxPagesTrip)
				}
				p.done[key] = true
				q := source.Query{Origin: home, Dest: dest, Out: pair.Out, Ret: pair.Ret, Cabin: p.t.Cabin, MaxStops: p.t.MaxStops, Airlines: p.t.Airlines}
				res, err := p.r.Cash.Search(ctx, q)
				p.calls++
				if err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if errors.Is(err, source.ErrBlocked) {
						return err
					}
					p.failures++
					p.lastErr = err
					log.Printf("scan %s: %s %s→%s %s/%s: %v", p.t.ID, tr.Name, home, dest, pair.Out, pair.Ret, err)
					if p.failures >= maxFailures {
						return fmt.Errorf("%d consecutive failures, last: %w", p.failures, err)
					}
					continue
				}
				p.failures = 0
				p.add(tr, q, res)
			}
		}
	}
	return nil
}

// add keeps the cheapest itinerary and, when different, the cheapest nonstop and the cheapest on each preferred airline.
func (p *priceRun) add(tr model.TripTraveler, q source.Query, res source.Result) {
	if len(res.Itineraries) == 0 {
		return
	}
	if p.t.ArriveBy != "" {
		res.Itineraries = arrivingBy(res.Itineraries, p.t.Depart, p.t.ArriveBy)
		if len(res.Itineraries) == 0 {
			return
		}
	}
	now := time.Now()
	mk := func(it source.Itinerary) model.Offer {
		return model.Offer{
			TripID: p.t.ID, TravelerID: tr.ID, Origin: q.Origin, Dest: q.Dest, OutDate: q.Out, RetDate: q.Ret,
			Kind: model.KindCash, PriceUSD: it.PriceUSD, Airline: it.Airline, Stops: it.Stops,
			DepartTime: it.Depart, ArriveTime: it.Arrive, ArriveOn: it.ArriveOn,
			PriceLevel: res.Insight.Level, TypicalLow: res.Insight.TypicalLow, TypicalHi: res.Insight.TypicalHi,
			SeenAt: now, LinkParams: it.Link,
		}
	}
	cheapest := res.Itineraries[0]
	p.offers = append(p.offers, mk(cheapest))
	nonstop := cheapest.Stops == 0
	seen := map[string]bool{cheapest.Airline: true} // cheapest per preferred airline, so rank order can be applied later
	for _, it := range res.Itineraries[1:] {
		keep := false
		if !nonstop && it.Stops == 0 {
			keep, nonstop = true, true
		}
		if tr.Prefers(it.Airline) && !seen[it.Airline] {
			keep, seen[it.Airline] = true, true
		}
		if keep {
			p.offers = append(p.offers, mk(it))
		}
	}
}

// arrivingBy keeps itineraries whose outbound lands on day by hhmm (string compare works on HH:MM).
func arrivingBy(its []source.Itinerary, day, hhmm string) []source.Itinerary {
	var out []source.Itinerary
	for _, it := range its {
		if it.ArriveOn == day && it.Arrive != "" && it.Arrive <= hhmm {
			out = append(out, it)
		}
	}
	return out
}

// Loop runs a scan every interval until ctx is done. The first run is immediate.
func (r *Runner) Loop(ctx context.Context, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		if err := r.Run(ctx, "", false); err != nil {
			log.Printf("scan: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
