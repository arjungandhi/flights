# flights design doc

Find cheap flights (cash or Amex points) for a group of travelers who may live in different cities, store what we're looking for, scan on a schedule, and push alerts through `messages` so the human books.

Non-goals: booking, seat selection, hotel/car, tracking booked itineraries for price-drop refunds.

# Trip Modes

Three ways to say what we want. One `trips` table; mode is derived from which fields are set.

| Mode | Destination | Dates | Example |
|---|---|---|---|
| `fixed` | known | exact depart + return, optional ±slack days | wedding in NRT, Mar 10–20 |
| `flexible` | known | window + trip length range | LIS, any 7–12 nights May–Aug |
| `explore` | open (optionally region-filtered) | window + trip length range | "somewhere cool", 4–6 nights, any weekend in Oct |

Every trip has 1..N travelers. Each traveler has a home airport set. A **candidate** is `(destination, out_date, ret_date)`; its cost is the sum of each traveler's cheapest offer for that date pair. All travelers must arrive at the destination on `out_date` and leave on `ret_date` (same calendar day; no tighter sync in v1).

# Process

One binary, no subcommands: `flights [--addr :8080] [--every 4h] [--notify "messages send"] [--dir $FLIGHTS_DIR] [--demo] [--no-scan]`. Serves the web UI and runs the scan loop on the timer. Each scan's alerts are piped as JSON lines (`{"room_id": ..., "text": ...}`) to the notify command's stdin (stdout when unset). Keys and valuations live in the Settings page, not flags or env, so partners can fix them. `--demo` seeds sample data into an empty DB; `--no-scan` serves the UI only.

Data dir `FLIGHTS_DIR` (default `~/.local/share/flights`), SQLite at `$FLIGHTS_DIR/flights.db`.

# Pricing Model

Everything is compared on one axis: **effective USD per person**.

- Cash offer: `effective = price_usd`
- Amex Travel pay-with-points is a cash fare at `1 / amex_cpp` points. Platinum 35% rebate on the selected airline (economy) or any airline (business/first) → `amex_cpp = 1.54`; otherwise `1.0`. Config: `amex_rebate_airline`, `amex_rebate_pct`. Never a separate offer; the UI prints "or N MR pts via Amex Travel" next to cash.
- Award offer: `effective = miles * cpp[program] / 100 + taxes_usd`. An award is **bookable** for a traveler only if `miles ≤ balance[program] + (balance[amex] if program is an MR transfer partner)`. Unbookable awards are stored but shown greyed and excluded from trip cost. Default `cpp` (editable in Settings):

  | Program | MR transfer | Books | Source | Default cpp |
  |---|---|---|---|---|
  | Delta SkyMiles | 1:1 | Delta | seats.aero | 1.2 |
  | Virgin Atlantic Flying Club | 1:1 | Delta (often fewer miles than SkyMiles), VS | seats.aero | 1.4 |
  | Air Canada Aeroplan | 1:1 | United + Star Alliance | seats.aero | 1.5 |
  | Flying Blue | 1:1 | AF/KLM, Delta, SkyTeam | seats.aero | 1.3 |
  | United MileagePlus | no | United | seats.aero | 1.2 |
  | Southwest Rapid Rewards | no | Southwest | derived from cash (see below) | 1.4 |

  Southwest: fares are on Google Flights (since mid-2024), so the cash side is free. Rapid Rewards pricing is revenue-based: `points ≈ base_fare / sw_rate` with `sw_rate` ≈ 0.014 for Wanna Get Away plus $5.60 TSA fee per segment. No seats.aero needed; `sw_rate` is a config value. seats.aero does not cover Southwest, so this derivation is the only award path for it.

  Programs a traveler can't reach (no balance and not MR-transferable) are skipped at scan time to save calls.
- Balances: set per traveler on the Travelers page. Manual in v1. Auto-pull is deliberately out: Delta/United/Southwest block logged-in scraping and AwardWallet has no personal API. A stale balance only mis-flags bookability, which the Book link lets you verify; alerts show balance `updated_at` so you know when to refresh.
- Trip cost = Σ over travelers of `min(cash, best bookable award)`. Offers are stored per traveler; the min is computed at query time so valuation or balance changes retroactively re-rank.

# Sources

Interface in `pkg/source`:

```go
type CashSource interface {
    Name() string
    // One route on one date pair, one adult. Implementations pace themselves.
    Search(ctx, Query{Origin, Dest, Out, Ret, Cabin, MaxStops, Airlines}) (Result{Itineraries, Insight}, error)
}

type AwardSource interface {
    Name() string
    Availability(ctx, origin, dest string, window DateRange, cabin) ([]AwardDay, error)
    Remaining() int
}
```

There is no calendar or explore call because no free provider offers one (below). Everything is built from `Search`.

## Cash: `google` (implemented)

Fetches the public Google Flights search page (`/travel/flights?tfs=…&curr=USD`) and reads the result payload it inlines (`AF_initDataCallback` key `ds:1`). Verified 2026-10-28 with plain Go `net/http` and a Chrome UA: no key, no TLS impersonation, no consent wall (cookie pre-set). Facts that shape the design:

- Google signature-gated its JSON RPCs (`GetShoppingResults`, `GetCalendarGraph`, `GetExploreDestinations`) in 2026-08. The HTML page is the only free path. The explore page inlines nothing; the date grid is not inlined either. So **one page = one (origin, dest, out, ret)**, 2–3 MB each.
- The page carries Google's **price insight** for that date pair: level (low/typical/high), typical range, and a ~60-day history. Stored on each offer (`price_level`, `typical_low`, `typical_high`); this is the baseline the explore rule uses instead of waiting weeks for our own history.
- Southwest fares are present. Basic-economy is not flagged, so there is no basic-economy filter.
- `tfs` is a protobuf: segments (date, origin, destination, max stops, carrier allow-list), cabin, passengers, trip type. Encoder in `pkg/source/google/tfs.go`, decoder in `parse.go`, both pinned by a live fixture test.
- Pacing: one page every 2.5–3.75 s (jittered), 3 attempts when a 200 arrives without `ds:1` (~1 in 60 is a transient variant). HTTP errors propagate. Run from a residential IP.

## Award: `seatsaero` (not yet)

Pro API ($9.99/mo, 1,000 calls/day). Cached Search gives per-day availability and mileage per cabin for a route across programs. Pro accounts do not get Live Search and not every Pro account gets API access at all, so check Settings → API before building on it. The key field exists in Settings; the source does not.

## Curated destinations

`pkg/geo/destinations.go`: ~180 airports tagged by region (`north-america`, `latam`, `europe`, `asia`, `oceania`, `africa`, `middle-east`). Explore mode fans `Search` out over this list, minus every traveler's home airports.

# Scan Algorithm

`pkg/scan`. One pass per active trip. Fixed trips scan every cycle (4h); flexible and explore trips are page-hungry and rescan once per ~20h unless "Scan now" forces it. Budgets are page counts, since pages are the cost:

1. **Enumerate** every allowed `(out, ret)` pair (`Pairs`): fixed = the ±slack grid; flexible/explore = every window date × nights range, weekends-only if set, returns inside the window, nothing before tomorrow.
2. **Coarse sample** evenly across the list (`Sample`): fixed up to 25 pairs, flexible 16, explore 3 per destination. Price every pair for every unbooked traveler from every home airport. Same pairs for everyone so candidates line up.
3. **Refine** around what is cheapest for the whole group (`pricing.Candidates` on what's priced so far): fixed/flexible price the ±2-day neighbours of the top 3 candidates at the same trip length; explore prices 6 more pairs for each of the top 5 destinations.
4. Hard cap 160 pages per trip per scan; 5 consecutive fetch failures abort; a page without results aborts immediately (rate limit or layout change).

Measured: fixed DEN+LGA+JFK→LAS = 3 pages, 8 s. Flexible 12-day window, 4–5 nights, 3 home airports = 45 pages, 137 s.

Per page we keep the cheapest itinerary and, if it had stops, the cheapest nonstop. Each scan writes to `scans` (trip, source, started, finished, calls_used, error); offers are append-only with `seen_at`; the UI reads the latest scan per trip. The lowest cash price per (origin, dest) per day goes to `price_history`.

# Alert Rules

Evaluated after each scan, per trip, on the ranked candidates:

1. **Under target**: per-person effective ≤ `target`. Deduped per `(trip, dest, out, ret)`; the same dates re-alert only on a further ≥10% drop.
2. **New low** (fixed/flexible): best candidate is ≥10% below the lowest price ever alerted for the trip, or nothing has been alerted yet (so the first scan always reports where things stand).
3. **Award space opened**: deferred until an award source exists.
4. **Explore deal** (explore mode without target): every traveler's fare sits at Google's "low" level for its dates.

Fixed trips send one message per candidate; flexible and explore send one batched message per scan with at most 5 candidates. Messages are JSON lines `{"room_id", "text"}` on the notify command's stdin; `room_id` and the base URL for links come from Settings.

```
✈ mex-apr · Mexico City (MEX) · 5 date option(s) under $400/pp
Mexico City (MEX) Apr 6–Apr 10 · $326/pp · $652 total
  Arjun: DEN $288 (VB 1 stop) · typical $275–360
  Sneha: JFK $364 (VB 1 stop) · typical $460–570
https://flights.tail/trips/mex-apr
```

Alerts are recorded in `alerts` before sending, so a crashed notify command loses at most one scan's messages rather than re-sending forever.

# Web UI

The only surface. Partners use it, so nothing requires a shell. Go `net/http` + `html/template` + htmx for partial updates; no JS build step. Written in Go, not a JS framework: the UI is tables and forms, and one `go build` ships the whole app. Pages:

- **Trips**: table of trips (mode, travelers, best effective price per person and total, Δ since last scan, last scanned, status). Row actions: pause/resume, delete. Add-trip form with three tabs matching the modes:
  - Destination: airport or metro code (`NRT`, `TYO`, `LON`) with typeahead; absent on the Explore tab, which has a region select (`anywhere`, `europe`, `asia`, `latam`, ...).
  - Dates: Fixed tab = depart + return pickers and ±slack days. Flexible/Explore tabs = window start/end, nights min–max, "weekends only" (Fri/Sat out, Sun/Mon back).
  - Travelers: checkboxes, all selected by default.
  - Cabin (economy/premium/business), target effective USD per person (required for fixed/flexible, optional for explore), max stops, airline allow-list, one-way.
  - Trip id is a short slug (`nrt-mar10`) used in URLs and alert text.
- **Trip detail**: ranked candidate table (dest, dates, total, per-traveler breakdown showing cash vs best bookable award and program, "or N MR via Amex Travel" next to cash), sparkline of lowest price per scan, alert history, "Book" links per traveler: Google Flights URL, airline/program award search prefilled with dates (Delta, Virgin Atlantic, Aeroplan, United, Flying Blue, Southwest), Amex Travel. Travelers are marked booked individually; the trip keeps scanning for the rest and stops when everyone is booked.
- **Explore**: deals board for explore-mode trips: destination cards with price per traveler, deal score vs baseline, best date pairs. The Flights-From-Home-style digest, but intersected across everyone's home airport.
- **Travelers**: add/remove; name, home airports (one or more; any is an acceptable origin), points balances per program (`delta`, `southwest`, `united`, `amex` = Membership Rewards) with inline edit and last-updated timestamps.
- **Settings**: seats.aero key (stored, unused until the award source lands), cpp table, Southwest rate, Amex rebate (airline + pct), notify command + Matrix room. Scan health: last 20 scans with calls used, errors, seats.aero calls remaining today. "Scan now" button.
- **Route history** (`/history/SFO/NRT`): lowest price per day over time; the baseline the explore deal score uses.


# Storage

SQLite, migrations embedded, same pattern as money.

- `travelers(id, name, homes TEXT /* comma list */)`
- `balances(traveler_id, program, miles, updated_at)` — `program` includes `amex`
- `trips(id TEXT PK, mode, dest, region, depart, ret, slack, window_start, window_end, nights_min, nights_max, weekends BOOL, one_way BOOL, cabin, target_usd, max_stops, airlines, status /* active|paused|booked */, created_at)`
- `trip_travelers(trip_id, traveler_id, booked BOOL)`
- `scans(id, trip_id, source, started_at, finished_at, calls_used, error)`
- `offers(id, scan_id, trip_id, traveler_id, origin, dest, out_date, ret_date, kind /* cash|award */, program, price_usd, miles, taxes_usd, airline, stops, basic_economy BOOL, price_level, typical_low, typical_high, seen_at, link_params /* google: tfs */)`
- `price_history(origin, dest, date /* scan day */, lowest_usd)` — one row per route per day, feeds the explore baseline
- `alerts(id, trip_id, dest, out_date, ret_date, program, effective_usd, sent_at)`
- `config(key, value)` — API keys, cpp table, amex rebate, matrix room id

# Deployment

- Go, modernc sqlite (no cgo), Nix flake + justfile like `messages`. Single binary serves the UI and runs the scanner.
- k8s Deployment on `hospital`, `nodeSelector: tree` (residential IP for Google), one replica, `FLIGHTS_DIR` on a PVC, secrets (seats.aero key, Matrix creds) via sops. Exposed on the tailnet only; partners get tailnet access via Tailscale node sharing. No app-level auth in v1.
- The scan loop runs the scan loop every 4h → ~6 runs/day. seats.aero: 1 call per route per window per run → ~10 trips × 2 origins × 6 ≈ 120/day, well under 1,000. Google: ~2–5 requests per trip per origin per run.
- Notification is still `messages`: `serve` pipes each alert batch to `--notify "messages send"`.
- Local use: same binary against a synced DB (Syncthing, same as `monkey/`), or just the UI over Tailscale.

# Risks

- Google page/protobuf drift breaks the cash source: shows up as `scans.error` on every trip (Settings → Scan health) and in the fixture test. SerpApi (~$75/mo) serves the same data if the page ever gets gated; `CashSource` is one interface so it would be a second implementation, not a rewrite.
- Explore is a brute-force fan-out (≈ destinations × 3 pairs × home airports pages), so a region-wide explore trip can hit the 160-page cap before refining. Narrow the region or window; a Google Explore replacement would need a browser session.
- seats.aero API access is not guaranteed to all Pro accounts and may be geo-restricted; check Settings → API before building on it. Without it the tool is cash-only.
- Award data is cached (hours old). Alerts say so; humans verify on the airline site.
- Multi-origin sync is same-day only. Good enough for v1; tighter arrival windows need per-itinerary times, which `Search` provides and could be added later.

# Later

- Nearby-airport expansion (`--nearby 100mi`).
- Positioning flights (fly cheap to a hub, then award).
- Price-drop tracking on booked trips for Delta/United refundable credits.
