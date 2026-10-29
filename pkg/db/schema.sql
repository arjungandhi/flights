PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS travelers (
    id    INTEGER PRIMARY KEY,
    name  TEXT NOT NULL UNIQUE,
    homes TEXT NOT NULL -- comma-separated airport codes
);

CREATE TABLE IF NOT EXISTS balances (
    traveler_id INTEGER NOT NULL REFERENCES travelers(id) ON DELETE CASCADE,
    program     TEXT NOT NULL,
    miles       INTEGER NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (traveler_id, program)
);

CREATE TABLE IF NOT EXISTS trips (
    id           TEXT PRIMARY KEY,
    mode         TEXT NOT NULL,
    dest         TEXT NOT NULL DEFAULT '',
    region       TEXT NOT NULL DEFAULT '',
    depart       TEXT NOT NULL DEFAULT '',
    ret          TEXT NOT NULL DEFAULT '',
    slack        INTEGER NOT NULL DEFAULT 0,
    window_start TEXT NOT NULL DEFAULT '',
    window_end   TEXT NOT NULL DEFAULT '',
    nights_min   INTEGER NOT NULL DEFAULT 0,
    nights_max   INTEGER NOT NULL DEFAULT 0,
    weekends     INTEGER NOT NULL DEFAULT 0,
    one_way      INTEGER NOT NULL DEFAULT 0,
    cabin        TEXT NOT NULL DEFAULT 'economy',
    target_usd   REAL NOT NULL DEFAULT 0,
    max_stops    INTEGER NOT NULL DEFAULT -1,
    airlines     TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'active',
    created_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS trip_travelers (
    trip_id     TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    traveler_id INTEGER NOT NULL REFERENCES travelers(id) ON DELETE CASCADE,
    booked      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (trip_id, traveler_id)
);

CREATE TABLE IF NOT EXISTS scans (
    id          INTEGER PRIMARY KEY,
    trip_id     TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    source      TEXT NOT NULL,
    started_at  TEXT NOT NULL,
    finished_at TEXT NOT NULL,
    calls_used  INTEGER NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS scans_trip ON scans(trip_id, started_at);

CREATE TABLE IF NOT EXISTS offers (
    id            INTEGER PRIMARY KEY,
    scan_id       INTEGER NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    trip_id       TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    traveler_id   INTEGER NOT NULL REFERENCES travelers(id) ON DELETE CASCADE,
    origin        TEXT NOT NULL,
    dest          TEXT NOT NULL,
    out_date      TEXT NOT NULL,
    ret_date      TEXT NOT NULL DEFAULT '',
    kind          TEXT NOT NULL,
    program       TEXT NOT NULL DEFAULT '',
    price_usd     REAL NOT NULL DEFAULT 0,
    miles         INTEGER NOT NULL DEFAULT 0,
    taxes_usd     REAL NOT NULL DEFAULT 0,
    airline       TEXT NOT NULL DEFAULT '',
    stops         INTEGER NOT NULL DEFAULT 0,
    basic_economy INTEGER NOT NULL DEFAULT 0,
    price_level   INTEGER NOT NULL DEFAULT 0,
    typical_low   REAL NOT NULL DEFAULT 0,
    typical_high  REAL NOT NULL DEFAULT 0,
    seen_at       TEXT NOT NULL,
    link_params   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS offers_trip ON offers(trip_id, scan_id);

CREATE TABLE IF NOT EXISTS price_history (
    origin     TEXT NOT NULL,
    dest       TEXT NOT NULL,
    date       TEXT NOT NULL, -- scan day
    lowest_usd REAL NOT NULL,
    PRIMARY KEY (origin, dest, date)
);

CREATE TABLE IF NOT EXISTS alerts (
    id            INTEGER PRIMARY KEY,
    trip_id       TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    dest          TEXT NOT NULL,
    out_date      TEXT NOT NULL,
    ret_date      TEXT NOT NULL DEFAULT '',
    program       TEXT NOT NULL DEFAULT '',
    effective_usd REAL NOT NULL,
    sent_at       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS alerts_trip ON alerts(trip_id, sent_at);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
