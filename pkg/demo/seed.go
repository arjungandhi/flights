// Package demo seeds an empty database with snehaple travelers, trips and offers so the UI
// can be exercised before any real source is wired up. Scan rows are tagged source "demo".
package demo

import (
	"fmt"
	"time"

	"github.com/arjungandhi/flights/pkg/db"
	"github.com/arjungandhi/flights/pkg/model"
)

func Seed(d *db.DB) error {
	ts, err := d.Travelers()
	if err != nil {
		return err
	}
	if len(ts) > 0 {
		return nil // already populated
	}
	now := time.Now()
	arjun, err := d.AddTraveler("Arjun", []string{"DEN"}, []string{"DL", "UA"})
	if err != nil {
		return err
	}
	sneha, err := d.AddTraveler("Sneha", []string{"LGA", "JFK"}, nil)
	if err != nil {
		return err
	}
	for _, b := range []struct {
		id      int64
		program string
		miles   int64
	}{
		{arjun, "amex", 250000}, {arjun, "delta", 82000}, {arjun, "united", 31000}, {arjun, "southwest", 44000},
		{sneha, "delta", 12000}, {sneha, "southwest", 90000},
	} {
		if err := d.SetBalance(b.id, b.program, b.miles, now.Add(-72*time.Hour)); err != nil {
			return err
		}
	}

	next := now.AddDate(0, 4, 0)
	dep := time.Date(next.Year(), next.Month(), 10, 0, 0, 0, 0, time.UTC)
	ret := dep.AddDate(0, 0, 10)
	fixed := model.Trip{Mode: model.ModeFixed, Dest: "NRT", Depart: day(dep), Return: day(ret), Slack: 1,
		Cabin: model.CabinEconomy, TargetUSD: 650, MaxStops: 1, Status: model.StatusActive, CreatedAt: now.Add(-20 * 24 * time.Hour)}
	if fixed.ID, err = d.Slug(fixed); err != nil {
		return err
	}
	if err := d.AddTrip(fixed, []int64{arjun, sneha}); err != nil {
		return err
	}

	ws := time.Date(next.Year(), next.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	flex := model.Trip{Mode: model.ModeFlexible, Dest: "LIS", WindowStart: day(ws), WindowEnd: day(ws.AddDate(0, 3, 0)), NightsMin: 7, NightsMax: 12,
		Cabin: model.CabinEconomy, TargetUSD: 550, MaxStops: -1, Status: model.StatusActive, CreatedAt: now.Add(-10 * 24 * time.Hour)}
	if flex.ID, err = d.Slug(flex); err != nil {
		return err
	}
	if err := d.AddTrip(flex, []int64{arjun, sneha}); err != nil {
		return err
	}

	explore := model.Trip{Mode: model.ModeExplore, Region: "latam", WindowStart: day(ws), WindowEnd: day(ws.AddDate(0, 2, 0)), NightsMin: 4, NightsMax: 6, Weekends: true,
		Cabin: model.CabinEconomy, MaxStops: 1, Status: model.StatusActive, CreatedAt: now.Add(-5 * 24 * time.Hour)}
	if explore.ID, err = d.Slug(explore); err != nil {
		return err
	}
	if err := d.AddTrip(explore, []int64{arjun, sneha}); err != nil {
		return err
	}

	// Three historical scans for the fixed trip so the sparkline has shape; the last one carries today's offers.
	for i, base := range []float64{689, 712, 598} {
		at := now.Add(-time.Duration(2-i) * 24 * time.Hour)
		scanID, err := d.AddScan(model.Scan{TripID: fixed.ID, Source: "demo", StartedAt: at, FinishedAt: at.Add(40 * time.Second), CallsUsed: 6})
		if err != nil {
			return err
		}
		offers := []model.Offer{
			{TravelerID: arjun, Origin: "DEN", Dest: "NRT", OutDate: day(dep), RetDate: day(ret), Kind: model.KindCash, PriceUSD: base, Airline: "UA", Stops: 0},
			{TravelerID: arjun, Origin: "DEN", Dest: "NRT", OutDate: day(dep), RetDate: day(ret), Kind: model.KindCash, PriceUSD: base - 61, Airline: "ZA", Stops: 0, BasicEconomy: true},
			{TravelerID: arjun, Origin: "DEN", Dest: "NRT", OutDate: day(dep), RetDate: day(ret), Kind: model.KindAward, Program: "delta", Miles: 70000, TaxesUSD: 62, Airline: "DL", Stops: 0},
			{TravelerID: arjun, Origin: "DEN", Dest: "NRT", OutDate: day(dep), RetDate: day(ret), Kind: model.KindAward, Program: "virginatlantic", Miles: 60000, TaxesUSD: 45, Airline: "DL", Stops: 0},
			{TravelerID: sneha, Origin: "LGA", Dest: "NRT", OutDate: day(dep), RetDate: day(ret), Kind: model.KindCash, PriceUSD: base + 190, Airline: "AA", Stops: 1},
			{TravelerID: sneha, Origin: "LGA", Dest: "NRT", OutDate: day(dep), RetDate: day(ret), Kind: model.KindAward, Program: "united", Miles: 88000, TaxesUSD: 40, Airline: "UA", Stops: 1},
			{TravelerID: arjun, Origin: "DEN", Dest: "NRT", OutDate: day(dep.AddDate(0, 0, 1)), RetDate: day(ret), Kind: model.KindCash, PriceUSD: base - 40, Airline: "UA", Stops: 0},
			{TravelerID: sneha, Origin: "LGA", Dest: "NRT", OutDate: day(dep.AddDate(0, 0, 1)), RetDate: day(ret), Kind: model.KindCash, PriceUSD: base + 240, Airline: "DL", Stops: 1},
		}
		for j := range offers {
			offers[j].ScanID, offers[j].TripID, offers[j].SeenAt = scanID, fixed.ID, at
		}
		if err := d.AddOffers(offers); err != nil {
			return err
		}
		if err := d.RecordPrice("DEN", "NRT", at, base-61); err != nil {
			return err
		}
		if err := d.RecordPrice("LGA", "NRT", at, base+190); err != nil {
			return err
		}
	}
	if err := d.AddAlert(model.Alert{TripID: fixed.ID, Dest: "NRT", OutDate: day(dep), RetDate: day(ret), EffectiveUSD: 598, SentAt: now.Add(-3 * time.Hour)}); err != nil {
		return err
	}

	// Flexible trip: a few date pairs.
	at := now.Add(-5 * time.Hour)
	scanID, err := d.AddScan(model.Scan{TripID: flex.ID, Source: "demo", StartedAt: at, FinishedAt: at.Add(2 * time.Minute), CallsUsed: 14})
	if err != nil {
		return err
	}
	var offers []model.Offer
	for i, pair := range [][2]int{{3, 11}, {17, 26}, {45, 53}} {
		out, back := day(ws.AddDate(0, 0, pair[0])), day(ws.AddDate(0, 0, pair[1]))
		offers = append(offers,
			model.Offer{TravelerID: arjun, Origin: "DEN", Dest: "LIS", OutDate: out, RetDate: back, Kind: model.KindCash, PriceUSD: 540 + float64(i*45), Airline: "TP", Stops: 0},
			model.Offer{TravelerID: arjun, Origin: "DEN", Dest: "LIS", OutDate: out, RetDate: back, Kind: model.KindAward, Program: "flyingblue", Miles: 55000, TaxesUSD: 180, Airline: "AF", Stops: 1},
			model.Offer{TravelerID: sneha, Origin: "LGA", Dest: "LIS", OutDate: out, RetDate: back, Kind: model.KindCash, PriceUSD: 610 + float64(i*30), Airline: "UA", Stops: 1},
			model.Offer{TravelerID: sneha, Origin: "LGA", Dest: "LIS", OutDate: out, RetDate: back, Kind: model.KindAward, Program: "aeroplan", Miles: 70000, TaxesUSD: 90, Airline: "UA", Stops: 1},
		)
	}
	for j := range offers {
		offers[j].ScanID, offers[j].TripID, offers[j].SeenAt = scanID, flex.ID, at
	}
	if err := d.AddOffers(offers); err != nil {
		return err
	}

	// Explore trip: destinations cheap from both SFO and AUS.
	scanID, err = d.AddScan(model.Scan{TripID: explore.ID, Source: "demo", StartedAt: at, FinishedAt: at.Add(3 * time.Minute), CallsUsed: 22})
	if err != nil {
		return err
	}
	offers = offers[:0]
	for _, dst := range []struct {
		code      string
		sfo, aus  float64
		swPoints  int64
		swAirline string
	}{
		{"MEX", 310, 280, 24000, "WN"}, {"SJO", 420, 390, 0, ""}, {"CUN", 380, 240, 19000, "WN"}, {"LIM", 560, 610, 0, ""},
	} {
		for k, pair := range [][2]int{{4, 9}, {18, 23}} {
			out, back := day(ws.AddDate(0, 0, pair[0])), day(ws.AddDate(0, 0, pair[1]))
			offers = append(offers,
				model.Offer{TravelerID: arjun, Origin: "DEN", Dest: dst.code, OutDate: out, RetDate: back, Kind: model.KindCash, PriceUSD: dst.sfo + float64(k*35), Airline: "AM", Stops: 0},
				model.Offer{TravelerID: sneha, Origin: "LGA", Dest: dst.code, OutDate: out, RetDate: back, Kind: model.KindCash, PriceUSD: dst.aus + float64(k*20), Airline: "AA", Stops: 1},
			)
			if dst.swPoints > 0 {
				offers = append(offers, model.Offer{TravelerID: sneha, Origin: "LGA", Dest: dst.code, OutDate: out, RetDate: back, Kind: model.KindAward, Program: "southwest", Miles: dst.swPoints, TaxesUSD: 11.2, Airline: dst.swAirline, Stops: 0})
			}
		}
	}
	for j := range offers {
		offers[j].ScanID, offers[j].TripID, offers[j].SeenAt = scanID, explore.ID, at
	}
	if err := d.AddOffers(offers); err != nil {
		return err
	}
	for i := range 30 {
		d0 := now.Add(-time.Duration(i) * 24 * time.Hour)
		if err := d.RecordPrice("DEN", "MEX", d0, 330+float64((i*37)%90)); err != nil {
			return err
		}
	}
	fmt.Println("demo: seeded travelers Arjun/Sneha and trips", fixed.ID, flex.ID, explore.ID)
	return nil
}

func day(t time.Time) string { return t.Format(model.DateLayout) }
