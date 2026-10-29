package db

import (
	"database/sql"
	"time"

	"github.com/arjungandhi/flights/pkg/model"
)

func (d *DB) AddScan(s model.Scan) (int64, error) {
	res, err := d.conn.Exec(`INSERT INTO scans(trip_id, source, started_at, finished_at, calls_used, error) VALUES(?,?,?,?,?,?)`,
		s.TripID, s.Source, fmtTime(s.StartedAt), fmtTime(s.FinishedAt), s.CallsUsed, s.Error)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) RecentScans(limit int) ([]model.Scan, error) {
	rows, err := d.conn.Query(`SELECT id, trip_id, source, started_at, finished_at, calls_used, error FROM scans ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Scan
	for rows.Next() {
		var s model.Scan
		var started, finished string
		if err := rows.Scan(&s.ID, &s.TripID, &s.Source, &started, &finished, &s.CallsUsed, &s.Error); err != nil {
			return nil, err
		}
		s.StartedAt, s.FinishedAt = parseTime(started), parseTime(finished)
		out = append(out, s)
	}
	return out, rows.Err()
}

// LastScan returns the most recent scan per trip id.
func (d *DB) LastScan() (map[string]model.Scan, error) {
	rows, err := d.conn.Query(`SELECT s.id, s.trip_id, s.source, s.started_at, s.finished_at, s.calls_used, s.error
		FROM scans s JOIN (SELECT trip_id, MAX(id) id FROM scans GROUP BY trip_id) m ON m.id = s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]model.Scan{}
	for rows.Next() {
		var s model.Scan
		var started, finished string
		if err := rows.Scan(&s.ID, &s.TripID, &s.Source, &started, &finished, &s.CallsUsed, &s.Error); err != nil {
			return nil, err
		}
		s.StartedAt, s.FinishedAt = parseTime(started), parseTime(finished)
		out[s.TripID] = s
	}
	return out, rows.Err()
}

func (d *DB) AddOffers(offers []model.Offer) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO offers(scan_id, trip_id, traveler_id, origin, dest, out_date, ret_date, kind, program,
		price_usd, miles, taxes_usd, airline, stops, basic_economy, price_level, typical_low, typical_high, seen_at, link_params) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, o := range offers {
		if _, err := stmt.Exec(o.ScanID, o.TripID, o.TravelerID, o.Origin, o.Dest, o.OutDate, o.RetDate, o.Kind, o.Program,
			o.PriceUSD, o.Miles, o.TaxesUSD, o.Airline, o.Stops, o.BasicEconomy, o.PriceLevel, o.TypicalLow, o.TypicalHi, fmtTime(o.SeenAt), o.LinkParams); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LatestOffers returns the offers from the newest scan that produced any offers for the trip.
func (d *DB) LatestOffers(tripID string) ([]model.Offer, error) {
	rows, err := d.conn.Query(`SELECT id, scan_id, trip_id, traveler_id, origin, dest, out_date, ret_date, kind, program,
		price_usd, miles, taxes_usd, airline, stops, basic_economy, price_level, typical_low, typical_high, seen_at, link_params
		FROM offers WHERE trip_id = ? AND scan_id = (SELECT MAX(scan_id) FROM offers WHERE trip_id = ?)`, tripID, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Offer
	for rows.Next() {
		var o model.Offer
		var seen string
		if err := rows.Scan(&o.ID, &o.ScanID, &o.TripID, &o.TravelerID, &o.Origin, &o.Dest, &o.OutDate, &o.RetDate, &o.Kind, &o.Program,
			&o.PriceUSD, &o.Miles, &o.TaxesUSD, &o.Airline, &o.Stops, &o.BasicEconomy, &o.PriceLevel, &o.TypicalLow, &o.TypicalHi, &seen, &o.LinkParams); err != nil {
			return nil, err
		}
		o.SeenAt = parseTime(seen)
		out = append(out, o)
	}
	return out, rows.Err()
}

// LowestPerScan is the cheapest cash price per scan for a trip, oldest first. Feeds the trip sparkline.
func (d *DB) LowestPerScan(tripID string) ([]model.PricePoint, error) {
	rows, err := d.conn.Query(`SELECT s.started_at, MIN(o.price_usd) FROM offers o JOIN scans s ON s.id = o.scan_id
		WHERE o.trip_id = ? AND o.kind = 'cash' GROUP BY o.scan_id ORDER BY s.started_at`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.PricePoint
	for rows.Next() {
		var p model.PricePoint
		if err := rows.Scan(&p.Date, &p.LowestUSD); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) RecordPrice(origin, dest string, day time.Time, lowest float64) error {
	_, err := d.conn.Exec(`INSERT INTO price_history(origin, dest, date, lowest_usd) VALUES(?,?,?,?)
		ON CONFLICT(origin, dest, date) DO UPDATE SET lowest_usd = MIN(lowest_usd, excluded.lowest_usd)`,
		origin, dest, day.Format(model.DateLayout), lowest)
	return err
}

func (d *DB) PriceHistory(origin, dest string) ([]model.PricePoint, error) {
	rows, err := d.conn.Query(`SELECT date, lowest_usd FROM price_history WHERE origin = ? AND dest = ? ORDER BY date`, origin, dest)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.PricePoint
	for rows.Next() {
		var p model.PricePoint
		if err := rows.Scan(&p.Date, &p.LowestUSD); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) AddAlert(a model.Alert) error {
	_, err := d.conn.Exec(`INSERT INTO alerts(trip_id, dest, out_date, ret_date, program, effective_usd, sent_at) VALUES(?,?,?,?,?,?,?)`,
		a.TripID, a.Dest, a.OutDate, a.RetDate, a.Program, a.EffectiveUSD, fmtTime(a.SentAt))
	return err
}

func (d *DB) Alerts(tripID string) ([]model.Alert, error) {
	rows, err := d.conn.Query(`SELECT id, trip_id, dest, out_date, ret_date, program, effective_usd, sent_at FROM alerts WHERE trip_id = ? ORDER BY sent_at DESC`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Alert
	for rows.Next() {
		var a model.Alert
		var sent string
		if err := rows.Scan(&a.ID, &a.TripID, &a.Dest, &a.OutDate, &a.RetDate, &a.Program, &a.EffectiveUSD, &sent); err != nil {
			return nil, err
		}
		a.SentAt = parseTime(sent)
		out = append(out, a)
	}
	return out, rows.Err()
}

// LastAlert returns the most recent alert for a trip on exact dates, any program.
func (d *DB) LastAlert(tripID, dest, out, ret string) (model.Alert, bool, error) {
	var a model.Alert
	var sent string
	err := d.conn.QueryRow(`SELECT id, trip_id, dest, out_date, ret_date, program, effective_usd, sent_at FROM alerts
		WHERE trip_id = ? AND dest = ? AND out_date = ? AND ret_date = ? ORDER BY sent_at DESC LIMIT 1`, tripID, dest, out, ret).
		Scan(&a.ID, &a.TripID, &a.Dest, &a.OutDate, &a.RetDate, &a.Program, &a.EffectiveUSD, &sent)
	if err != nil {
		if scanOne(err) == ErrNotFound {
			return a, false, nil
		}
		return a, false, err
	}
	a.SentAt = parseTime(sent)
	return a, true, nil
}

// MinAlerted is the lowest per-person effective price ever alerted for a trip; ok=false when none.
func (d *DB) MinAlerted(tripID string) (float64, bool, error) {
	var v sql.NullFloat64
	if err := d.conn.QueryRow(`SELECT MIN(effective_usd) FROM alerts WHERE trip_id = ?`, tripID).Scan(&v); err != nil {
		return 0, false, err
	}
	return v.Float64, v.Valid, nil
}
