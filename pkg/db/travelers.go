package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/arjungandhi/flights/pkg/model"
)

var ErrNotFound = errors.New("not found")

func (d *DB) Travelers() ([]model.Traveler, error) {
	rows, err := d.conn.Query(`SELECT id, name, homes FROM travelers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Traveler
	for rows.Next() {
		var t model.Traveler
		var homes string
		if err := rows.Scan(&t.ID, &t.Name, &homes); err != nil {
			return nil, err
		}
		t.Homes = splitList(homes)
		t.Balances = map[string]model.Balance{}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, d.loadBalances(out)
}

func (d *DB) loadBalances(ts []model.Traveler) error {
	idx := map[int64]int{}
	for i, t := range ts {
		idx[t.ID] = i
	}
	rows, err := d.conn.Query(`SELECT traveler_id, program, miles, updated_at FROM balances`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var program, updated string
		var b model.Balance
		if err := rows.Scan(&id, &program, &b.Miles, &updated); err != nil {
			return err
		}
		b.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		if i, ok := idx[id]; ok {
			ts[i].Balances[program] = b
		}
	}
	return rows.Err()
}

func (d *DB) AddTraveler(name string, homes []string) (int64, error) {
	res, err := d.conn.Exec(`INSERT INTO travelers(name, homes) VALUES(?, ?)`, name, joinList(homes))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdateTraveler(id int64, name string, homes []string) error {
	_, err := d.conn.Exec(`UPDATE travelers SET name = ?, homes = ? WHERE id = ?`, name, joinList(homes), id)
	return err
}

func (d *DB) DeleteTraveler(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM travelers WHERE id = ?`, id)
	return err
}

func (d *DB) SetBalance(travelerID int64, program string, miles int64, at time.Time) error {
	_, err := d.conn.Exec(`INSERT INTO balances(traveler_id, program, miles, updated_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(traveler_id, program) DO UPDATE SET miles = excluded.miles, updated_at = excluded.updated_at`,
		travelerID, program, miles, at.UTC().Format(time.RFC3339))
	return err
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}

func joinList(xs []string) string { return strings.Join(xs, ",") }

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func scanOne(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
