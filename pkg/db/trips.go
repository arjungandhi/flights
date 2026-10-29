package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/arjungandhi/flights/pkg/model"
)

const tripCols = `id, mode, dest, region, depart, ret, slack, arrive_by, window_start, window_end, nights_min, nights_max,
	weekends, one_way, cabin, target_usd, max_stops, airlines, status, created_at`

func scanTrip(sc interface{ Scan(...any) error }) (model.Trip, error) {
	var t model.Trip
	var airlines, created string
	err := sc.Scan(&t.ID, &t.Mode, &t.Dest, &t.Region, &t.Depart, &t.Return, &t.Slack, &t.ArriveBy, &t.WindowStart, &t.WindowEnd,
		&t.NightsMin, &t.NightsMax, &t.Weekends, &t.OneWay, &t.Cabin, &t.TargetUSD, &t.MaxStops, &airlines,
		&t.Status, &created)
	if err != nil {
		return t, scanOne(err)
	}
	t.Airlines = splitList(airlines)
	t.CreatedAt = parseTime(created)
	return t, nil
}

// Trips returns all trips with their travelers, newest first.
func (d *DB) Trips() ([]model.Trip, error) {
	rows, err := d.conn.Query(`SELECT ` + tripCols + ` FROM trips ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Trip
	for rows.Next() {
		t, err := scanTrip(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, d.attachTravelers(out)
}

func (d *DB) Trip(id string) (model.Trip, error) {
	t, err := scanTrip(d.conn.QueryRow(`SELECT `+tripCols+` FROM trips WHERE id = ?`, id))
	if err != nil {
		return t, err
	}
	ts := []model.Trip{t}
	if err := d.attachTravelers(ts); err != nil {
		return t, err
	}
	return ts[0], nil
}

func (d *DB) attachTravelers(trips []model.Trip) error {
	travelers, err := d.Travelers()
	if err != nil {
		return err
	}
	byID := map[int64]model.Traveler{}
	for _, t := range travelers {
		byID[t.ID] = t
	}
	rows, err := d.conn.Query(`SELECT trip_id, traveler_id, booked FROM trip_travelers`)
	if err != nil {
		return err
	}
	defer rows.Close()
	members := map[string][]model.TripTraveler{}
	for rows.Next() {
		var tripID string
		var tid int64
		var booked bool
		if err := rows.Scan(&tripID, &tid, &booked); err != nil {
			return err
		}
		if tr, ok := byID[tid]; ok {
			members[tripID] = append(members[tripID], model.TripTraveler{Traveler: tr, Booked: booked})
		}
	}
	for i := range trips {
		trips[i].Travelers = members[trips[i].ID]
	}
	return rows.Err()
}

// AddTrip inserts the trip and its travelers. t.ID must already be a unique slug; see Slug.
func (d *DB) AddTrip(t model.Trip, travelerIDs []int64) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO trips(`+tripCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Mode, t.Dest, t.Region, t.Depart, t.Return, t.Slack, t.ArriveBy, t.WindowStart, t.WindowEnd, t.NightsMin, t.NightsMax,
		t.Weekends, t.OneWay, t.Cabin, t.TargetUSD, t.MaxStops, joinList(t.Airlines), t.Status, fmtTime(t.CreatedAt))
	if err != nil {
		return err
	}
	for _, id := range travelerIDs {
		if _, err := tx.Exec(`INSERT INTO trip_travelers(trip_id, traveler_id) VALUES(?, ?)`, t.ID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Slug picks a unique id like "nrt-mar10" or "explore-europe-may".
func (d *DB) Slug(t model.Trip) (string, error) {
	var base string
	when := t.Depart
	if t.Mode != model.ModeFixed {
		when = t.WindowStart
	}
	tm, _ := time.Parse(model.DateLayout, when)
	switch t.Mode {
	case model.ModeFixed:
		base = fmt.Sprintf("%s-%s", strings.ToLower(t.Dest), strings.ToLower(tm.Format("Jan2")))
	case model.ModeFlexible:
		base = fmt.Sprintf("%s-%s", strings.ToLower(t.Dest), strings.ToLower(tm.Format("Jan")))
	default:
		base = fmt.Sprintf("explore-%s-%s", t.Where(), strings.ToLower(tm.Format("Jan")))
	}
	id := base
	for n := 2; ; n++ {
		var exists int
		if err := d.conn.QueryRow(`SELECT COUNT(*) FROM trips WHERE id = ?`, id).Scan(&exists); err != nil {
			return "", err
		}
		if exists == 0 {
			return id, nil
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

func (d *DB) SetTripStatus(id string, s model.Status) error {
	_, err := d.conn.Exec(`UPDATE trips SET status = ? WHERE id = ?`, s, id)
	return err
}

func (d *DB) DeleteTrip(id string) error {
	_, err := d.conn.Exec(`DELETE FROM trips WHERE id = ?`, id)
	return err
}

// SetBooked marks one traveler booked on a trip. When everyone is booked the trip status becomes booked.
func (d *DB) SetBooked(tripID string, travelerID int64, booked bool) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE trip_travelers SET booked = ? WHERE trip_id = ? AND traveler_id = ?`, booked, tripID, travelerID); err != nil {
		return err
	}
	var remaining int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM trip_travelers WHERE trip_id = ? AND booked = 0`, tripID).Scan(&remaining); err != nil {
		return err
	}
	status := model.StatusActive
	if remaining == 0 {
		status = model.StatusBooked
	}
	if _, err := tx.Exec(`UPDATE trips SET status = ? WHERE id = ? AND status != 'paused'`, status, tripID); err != nil {
		return err
	}
	return tx.Commit()
}
