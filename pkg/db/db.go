// Package db is the SQLite storage layer. One file, WAL mode, schema applied on open.
package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/arjungandhi/flights/pkg/model"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type DB struct {
	conn *sql.DB
}

// DefaultDir is $FLIGHTS_DIR or ~/.local/share/flights.
func DefaultDir() string {
	if d := os.Getenv("FLIGHTS_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "flights-data"
	}
	return filepath.Join(home, ".local", "share", "flights")
}

func Open(dir string) (*DB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	conn, err := sql.Open("sqlite", filepath.Join(dir, "flights.db"))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// The scan loop and HTTP handlers share this; sqlite serializes writers itself.
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	// Columns added after the first release. ADD COLUMN is idempotent here because a
	// "duplicate column" error is the only failure mode and is ignored.
	for _, stmt := range []string{
		`ALTER TABLE travelers ADD COLUMN airlines TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE trips ADD COLUMN arrive_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE offers ADD COLUMN depart_time TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE offers ADD COLUMN arrive_time TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE offers ADD COLUMN arrive_on TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE offers ADD COLUMN price_level INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE offers ADD COLUMN typical_low REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE offers ADD COLUMN typical_high REAL NOT NULL DEFAULT 0`,
	} {
		if _, err := conn.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			conn.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	return &DB{conn: conn}, nil
}

func (d *DB) Close() error { return d.conn.Close() }

// Settings is the typed view of the settings table.
type Settings struct {
	SeatsAeroKey      string
	CPP               map[string]float64
	SWRate            float64 // dollars per Southwest point
	AmexRebateAirline string
	AmexRebatePct     float64
	PreferPct         float64 // a preferred-airline fare wins when within this % of the cheapest
	NotifyRoom        string
	BaseURL           string // used in alert text links
}

func DefaultSettings() Settings {
	s := Settings{
		CPP:           map[string]float64{},
		SWRate:        0.014,
		AmexRebatePct: 35,
		PreferPct:     10,
	}
	for _, p := range model.Programs {
		s.CPP[p.ID] = p.DefaultCPP
	}
	return s
}

// AmexCPP is the cents-per-point value of MR spent through Amex Travel with the rebate applied.
func (s Settings) AmexCPP() float64 {
	if s.AmexRebatePct <= 0 || s.AmexRebatePct >= 100 {
		return 1
	}
	return 1 / (1 - s.AmexRebatePct/100)
}

func (d *DB) Settings() (Settings, error) {
	s := DefaultSettings()
	rows, err := d.conn.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return s, err
		}
		switch k {
		case "seatsaero_key":
			s.SeatsAeroKey = v
		case "sw_rate":
			s.SWRate, _ = strconv.ParseFloat(v, 64)
		case "amex_rebate_airline":
			s.AmexRebateAirline = v
		case "amex_rebate_pct":
			s.AmexRebatePct, _ = strconv.ParseFloat(v, 64)
		case "prefer_pct":
			s.PreferPct, _ = strconv.ParseFloat(v, 64)
		case "notify_room":
			s.NotifyRoom = v
		case "base_url":
			s.BaseURL = v
		default:
			if len(k) > 4 && k[:4] == "cpp_" {
				s.CPP[k[4:]], _ = strconv.ParseFloat(v, 64)
			}
		}
	}
	return s, rows.Err()
}

func (d *DB) SaveSettings(s Settings) error {
	kv := map[string]string{
		"seatsaero_key":       s.SeatsAeroKey,
		"sw_rate":             strconv.FormatFloat(s.SWRate, 'f', -1, 64),
		"amex_rebate_airline": s.AmexRebateAirline,
		"amex_rebate_pct":     strconv.FormatFloat(s.AmexRebatePct, 'f', -1, 64),
		"prefer_pct":          strconv.FormatFloat(s.PreferPct, 'f', -1, 64),
		"notify_room":         s.NotifyRoom,
		"base_url":            s.BaseURL,
	}
	for id, v := range s.CPP {
		kv["cpp_"+id] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range kv {
		if _, err := tx.Exec(`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}
