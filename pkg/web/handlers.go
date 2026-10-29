package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/arjungandhi/flights/pkg/db"
	"github.com/arjungandhi/flights/pkg/model"
	"github.com/arjungandhi/flights/pkg/pricing"
)

// ---- Trips ----

type tripRow struct {
	model.Trip
	Best     *pricing.Candidate
	LastScan model.Scan
}

type tripsPage struct {
	Trips     []tripRow
	Travelers []model.Traveler
	Cabins    []model.Cabin
	Regions   []string
	Form      map[string]string // echoed back on validation error
}

func (s *Server) tripsData() (tripsPage, error) {
	trips, err := s.DB.Trips()
	if err != nil {
		return tripsPage{}, err
	}
	travelers, err := s.DB.Travelers()
	if err != nil {
		return tripsPage{}, err
	}
	settings, err := s.DB.Settings()
	if err != nil {
		return tripsPage{}, err
	}
	last, err := s.DB.LastScan()
	if err != nil {
		return tripsPage{}, err
	}
	p := tripsPage{Travelers: travelers, Cabins: model.Cabins, Regions: model.Regions, Form: map[string]string{}}
	for _, t := range trips {
		row := tripRow{Trip: t, LastScan: last[t.ID]}
		offers, err := s.DB.LatestOffers(t.ID)
		if err != nil {
			return p, err
		}
		if c := pricing.Candidates(t, offers, settings); len(c) > 0 {
			row.Best = &c[0]
		}
		p.Trips = append(p.Trips, row)
	}
	return p, nil
}

func (s *Server) trips(w http.ResponseWriter, r *http.Request) {
	data, err := s.tripsData()
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, "trips", page{Title: "Trips", Data: data, Flash: r.URL.Query().Get("flash")})
}

func (s *Server) addTrip(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	t, ids, err := parseTripForm(r)
	if err == nil {
		t.ID, err = s.DB.Slug(t)
	}
	if err == nil {
		err = s.DB.AddTrip(t, ids)
	}
	if err != nil {
		data, derr := s.tripsData()
		if derr != nil {
			s.fail(w, derr)
			return
		}
		for k, v := range r.PostForm {
			data.Form[k] = strings.Join(v, ",")
		}
		s.render(w, "trips", page{Title: "Trips", Error: err.Error(), Data: data})
		return
	}
	http.Redirect(w, r, "/trips/"+t.ID, http.StatusSeeOther)
}

func parseTripForm(r *http.Request) (model.Trip, []int64, error) {
	f := r.PostForm
	t := model.Trip{
		Mode:      model.Mode(f.Get("mode")),
		Dest:      strings.ToUpper(strings.TrimSpace(f.Get("dest"))),
		Region:    f.Get("region"),
		Cabin:     model.Cabin(f.Get("cabin")),
		Status:    model.StatusActive,
		CreatedAt: time.Now(),
		OneWay:    f.Get("one_way") == "on",
		Weekends:  f.Get("weekends") == "on",
		MaxStops:  -1,
	}
	if t.Cabin == "" {
		t.Cabin = model.CabinEconomy
	}
	if v := f.Get("target_usd"); v != "" {
		x, err := strconv.ParseFloat(v, 64)
		if err != nil || x < 0 {
			return t, nil, errors.New("target must be a dollar amount")
		}
		t.TargetUSD = x
	}
	if v := f.Get("max_stops"); v != "" && v != "any" {
		t.MaxStops, _ = strconv.Atoi(v)
	}
	for _, a := range strings.Split(f.Get("airlines"), ",") {
		if a = strings.ToUpper(strings.TrimSpace(a)); a != "" {
			t.Airlines = append(t.Airlines, a)
		}
	}
	for _, v := range f["travelers"] {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return t, nil, errors.New("bad traveler id")
		}
		t.Travelers = append(t.Travelers, model.TripTraveler{Traveler: model.Traveler{ID: id}})
	}
	if len(t.Travelers) == 0 {
		return t, nil, errors.New("pick at least one traveler")
	}
	ids := make([]int64, len(t.Travelers))
	for i, tt := range t.Travelers {
		ids[i] = tt.ID
	}

	switch t.Mode {
	case model.ModeFixed:
		if len(t.Dest) != 3 {
			return t, nil, errors.New("destination must be a 3-letter airport or metro code")
		}
		t.Depart = f.Get("depart")
		t.Return = f.Get("return")
		if !validDate(t.Depart) {
			return t, nil, errors.New("departure date required")
		}
		if t.OneWay {
			t.Return = ""
		} else if !validDate(t.Return) || t.Return < t.Depart {
			return t, nil, errors.New("return date must be on or after departure")
		}
		t.Slack, _ = strconv.Atoi(f.Get("slack"))
		if t.Slack < 0 || t.Slack > 7 {
			return t, nil, errors.New("slack must be 0–7 days")
		}
		if t.ArriveBy = f.Get("arrive_by"); t.ArriveBy != "" {
			if _, err := time.Parse("15:04", t.ArriveBy); err != nil {
				return t, nil, errors.New("arrive by must be HH:MM")
			}
		}
		if t.TargetUSD == 0 {
			return t, nil, errors.New("target price is required for fixed trips")
		}
	case model.ModeFlexible, model.ModeExplore:
		if t.Mode == model.ModeFlexible && len(t.Dest) != 3 {
			return t, nil, errors.New("destination must be a 3-letter airport or metro code")
		}
		if t.Mode == model.ModeExplore {
			t.Dest = ""
			if t.Region == "" {
				t.Region = "anywhere"
			}
		}
		t.WindowStart, t.WindowEnd = f.Get("window_start"), f.Get("window_end")
		if !validDate(t.WindowStart) || !validDate(t.WindowEnd) || t.WindowEnd < t.WindowStart {
			return t, nil, errors.New("window end must be on or after window start")
		}
		if !t.OneWay {
			t.NightsMin, _ = strconv.Atoi(f.Get("nights_min"))
			t.NightsMax, _ = strconv.Atoi(f.Get("nights_max"))
			if t.NightsMin < 1 || t.NightsMax < t.NightsMin {
				return t, nil, errors.New("nights range must be at least 1 and min ≤ max")
			}
		}
		if t.Mode == model.ModeFlexible && t.TargetUSD == 0 {
			return t, nil, errors.New("target price is required for flexible trips")
		}
	default:
		return t, nil, errors.New("unknown trip mode")
	}
	return t, ids, nil
}

func validDate(s string) bool {
	_, err := time.Parse(model.DateLayout, s)
	return err == nil
}

// ---- Trip detail ----

type tripPage struct {
	Trip       model.Trip
	Candidates []pricing.Candidate
	Alerts     []model.Alert
	Spark      []model.PricePoint
	LastScan   model.Scan
	Settings   db.Settings
}

func (s *Server) trip(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.DB.Trip(id)
	if errors.Is(err, db.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	settings, err := s.DB.Settings()
	if err != nil {
		s.fail(w, err)
		return
	}
	offers, err := s.DB.LatestOffers(id)
	if err != nil {
		s.fail(w, err)
		return
	}
	alerts, err := s.DB.Alerts(id)
	if err != nil {
		s.fail(w, err)
		return
	}
	spark, err := s.DB.LowestPerScan(id)
	if err != nil {
		s.fail(w, err)
		return
	}
	last, err := s.DB.LastScan()
	if err != nil {
		s.fail(w, err)
		return
	}
	cands := pricing.Candidates(t, offers, settings)
	if len(cands) > 20 {
		cands = cands[:20]
	}
	s.render(w, "trip", page{Title: id, Active: "trips", Data: tripPage{
		Trip: t, Candidates: cands, Alerts: alerts, Spark: spark, LastScan: last[id], Settings: settings,
	}})
}

func (s *Server) tripStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	st := model.Status(r.FormValue("status"))
	if st != model.StatusActive && st != model.StatusPaused {
		http.Error(w, "bad status", http.StatusBadRequest)
		return
	}
	if err := s.DB.SetTripStatus(id, st); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}

func (s *Server) deleteTrip(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.DeleteTrip(r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) booked(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tid, err := strconv.ParseInt(r.FormValue("traveler"), 10, 64)
	if err != nil {
		http.Error(w, "bad traveler", http.StatusBadRequest)
		return
	}
	if err := s.DB.SetBooked(id, tid, r.FormValue("booked") == "1"); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, "/trips/"+id, http.StatusSeeOther)
}

// ---- Explore ----

type exploreDest struct {
	Dest  string
	Cands []pricing.Candidate
}

type exploreTrip struct {
	model.Trip
	Dests    []exploreDest
	LastScan model.Scan
}

func (s *Server) explore(w http.ResponseWriter, r *http.Request) {
	trips, err := s.DB.Trips()
	if err != nil {
		s.fail(w, err)
		return
	}
	settings, err := s.DB.Settings()
	if err != nil {
		s.fail(w, err)
		return
	}
	last, err := s.DB.LastScan()
	if err != nil {
		s.fail(w, err)
		return
	}
	var out []exploreTrip
	for _, t := range trips {
		if t.Mode != model.ModeExplore {
			continue
		}
		offers, err := s.DB.LatestOffers(t.ID)
		if err != nil {
			s.fail(w, err)
			return
		}
		et := exploreTrip{Trip: t, LastScan: last[t.ID]}
		byDest := map[string]*exploreDest{}
		for _, c := range pricing.Candidates(t, offers, settings) {
			d := byDest[c.Dest]
			if d == nil {
				d = &exploreDest{Dest: c.Dest}
				byDest[c.Dest] = d
				et.Dests = append(et.Dests, exploreDest{})
			}
			if len(d.Cands) < 3 {
				d.Cands = append(d.Cands, c)
			}
		}
		et.Dests = et.Dests[:0]
		for _, d := range byDest {
			et.Dests = append(et.Dests, *d)
		}
		sort.Slice(et.Dests, func(i, j int) bool { return et.Dests[i].Cands[0].Total < et.Dests[j].Cands[0].Total })
		out = append(out, et)
	}
	s.render(w, "explore", page{Title: "Explore", Data: out})
}

// ---- Travelers ----

type travelersPage struct {
	Travelers []model.Traveler
	Programs  []model.Program
}

func (s *Server) travelers(w http.ResponseWriter, r *http.Request) {
	ts, err := s.DB.Travelers()
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, "travelers", page{Title: "Travelers", Data: travelersPage{Travelers: ts, Programs: model.BalancePrograms}, Error: r.URL.Query().Get("error")})
}

func parseHomes(s string) ([]string, error) {
	var homes []string
	for _, h := range strings.Split(s, ",") {
		h = strings.ToUpper(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if len(h) != 3 {
			return nil, fmt.Errorf("%q is not a 3-letter airport code", h)
		}
		homes = append(homes, h)
	}
	if len(homes) == 0 {
		return nil, errors.New("at least one home airport is required")
	}
	return homes, nil
}

func parseAirlines(s string) []string {
	var out []string
	for _, a := range strings.Split(s, ",") {
		if a = strings.ToUpper(strings.TrimSpace(a)); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func (s *Server) addTraveler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	homes, err := parseHomes(r.FormValue("homes"))
	if name == "" {
		err = errors.New("name is required")
	}
	if err == nil {
		_, err = s.DB.AddTraveler(name, homes, parseAirlines(r.FormValue("airlines")))
	}
	if err != nil {
		http.Redirect(w, r, "/travelers?error="+errString(err), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/travelers", http.StatusSeeOther)
}

func (s *Server) updateTraveler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	name := strings.TrimSpace(r.FormValue("name"))
	homes, err := parseHomes(r.FormValue("homes"))
	if name == "" {
		err = errors.New("name is required")
	}
	if err == nil {
		err = s.DB.UpdateTraveler(id, name, homes, parseAirlines(r.FormValue("airlines")))
	}
	if err != nil {
		http.Redirect(w, r, "/travelers?error="+errString(err), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/travelers", http.StatusSeeOther)
}

func (s *Server) deleteTraveler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.DB.DeleteTraveler(id); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, "/travelers", http.StatusSeeOther)
}

// setBalances writes every submitted program whose value changed; untouched fields keep their timestamp.
func (s *Server) setBalances(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	ts, err := s.DB.Travelers()
	if err != nil {
		s.fail(w, err)
		return
	}
	var cur map[string]model.Balance
	for _, t := range ts {
		if t.ID == id {
			cur = t.Balances
		}
	}
	now := time.Now()
	for _, p := range model.BalancePrograms {
		v := strings.ReplaceAll(strings.TrimSpace(r.PostForm.Get("balance_"+p.ID)), ",", "")
		if v == "" {
			continue
		}
		miles, err := strconv.ParseInt(v, 10, 64)
		if err != nil || miles < 0 {
			http.Redirect(w, r, "/travelers?error="+errString(fmt.Errorf("%s balance must be a whole number", p.Name)), http.StatusSeeOther)
			return
		}
		if b, ok := cur[p.ID]; ok && b.Miles == miles {
			continue
		}
		if err := s.DB.SetBalance(id, p.ID, miles, now); err != nil {
			s.fail(w, err)
			return
		}
	}
	http.Redirect(w, r, "/travelers", http.StatusSeeOther)
}

func errString(err error) string { return strings.ReplaceAll(err.Error(), " ", "+") }

// ---- Settings ----

type settingsPage struct {
	Settings  db.Settings
	Programs  []model.Program
	Scans     []model.Scan
	Remaining int
	Every     time.Duration
	Sources   string
}

func (s *Server) settingsData() (settingsPage, error) {
	settings, err := s.DB.Settings()
	if err != nil {
		return settingsPage{}, err
	}
	scans, err := s.DB.RecentScans(20)
	if err != nil {
		return settingsPage{}, err
	}
	p := settingsPage{Settings: settings, Programs: model.Programs, Scans: scans, Remaining: -1, Every: s.Every, Sources: "none"}
	if s.Runner.Award != nil {
		p.Remaining = s.Runner.Award.Remaining()
	}
	var names []string
	if s.Runner.Cash != nil {
		names = append(names, s.Runner.Cash.Name())
	}
	if s.Runner.Award != nil {
		names = append(names, s.Runner.Award.Name())
	}
	if len(names) > 0 {
		p.Sources = strings.Join(names, ", ")
	}
	return p, nil
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	data, err := s.settingsData()
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, "settings", page{Title: "Settings", Data: data, Flash: r.URL.Query().Get("flash"), Error: r.URL.Query().Get("error")})
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	cur, err := s.DB.Settings()
	if err != nil {
		s.fail(w, err)
		return
	}
	f := r.PostForm
	cur.SeatsAeroKey = strings.TrimSpace(f.Get("seatsaero_key"))
	cur.AmexRebateAirline = strings.ToUpper(strings.TrimSpace(f.Get("amex_rebate_airline")))
	cur.NotifyRoom = strings.TrimSpace(f.Get("notify_room"))
	cur.BaseURL = strings.TrimRight(strings.TrimSpace(f.Get("base_url")), "/")
	num := func(key string, lo, hi float64) (float64, error) {
		v, err := strconv.ParseFloat(strings.TrimSpace(f.Get(key)), 64)
		if err != nil || v < lo || v > hi {
			return 0, fmt.Errorf("%s must be between %g and %g", key, lo, hi)
		}
		return v, nil
	}
	if cur.AmexRebatePct, err = num("amex_rebate_pct", 0, 99); err == nil {
		cur.SWRate, err = num("sw_rate", 0.001, 0.1)
	}
	if err == nil {
		cur.PreferPct, err = num("prefer_pct", 0, 100)
	}
	for _, p := range model.Programs {
		if err != nil {
			break
		}
		cur.CPP[p.ID], err = num("cpp_"+p.ID, 0.1, 10)
	}
	if err == nil {
		err = s.DB.SaveSettings(cur)
	}
	if err != nil {
		http.Redirect(w, r, "/settings?error="+errString(err), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?flash=saved", http.StatusSeeOther)
}

// scanNow starts a forced pass in the background (explore scans run for many minutes) and
// returns the scan-health partial for htmx. Results land in the scans table when done.
func (s *Server) scanNow(w http.ResponseWriter, r *http.Request) {
	tripID := r.FormValue("trip")
	if s.Runner.Busy() {
		s.scanResult(w, r, "A scan is already running; results appear in Scan health when it finishes.")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		if err := s.Runner.Run(ctx, tripID, true); err != nil {
			log.Printf("scan now: %v", err)
		}
	}()
	time.Sleep(300 * time.Millisecond) // let a trivially failing scan record itself before we render
	s.scanResult(w, r, "Scan started. Refresh in a bit; each page takes ~3 s.")
}

func (s *Server) scanResult(w http.ResponseWriter, r *http.Request, flash string) {
	data, err := s.settingsData()
	if err != nil {
		s.fail(w, err)
		return
	}
	p := page{Data: data, Flash: flash}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.pages["settings"].ExecuteTemplate(w, "scans", p); err != nil {
			s.fail(w, err)
		}
		return
	}
	http.Redirect(w, r, "/settings?flash="+errString(errors.New(flash)), http.StatusSeeOther)
}

// ---- History ----

type historyPage struct {
	Origin, Dest string
	Points       []model.PricePoint
	Min, Median  float64
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	origin, dest := strings.ToUpper(r.PathValue("origin")), strings.ToUpper(r.PathValue("dest"))
	pts, err := s.DB.PriceHistory(origin, dest)
	if err != nil {
		s.fail(w, err)
		return
	}
	p := historyPage{Origin: origin, Dest: dest, Points: pts}
	if len(pts) > 0 {
		vals := make([]float64, len(pts))
		for i, pt := range pts {
			vals[i] = pt.LowestUSD
		}
		sort.Float64s(vals)
		p.Min, p.Median = vals[0], vals[len(vals)/2]
	}
	s.render(w, "history", page{Title: origin + " → " + dest, Active: "explore", Data: p})
}
