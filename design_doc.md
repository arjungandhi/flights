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
    // Lowest price per date for a route over a window. One call per (origin, dest, month) ideally.
    Calendar(ctx, origin, dest string, window DateRange, opts) ([]DatePrice, error)
    // Itineraries for a specific date pair (only for shortlisted candidates).
    Search(ctx, origin, dest string, out, ret Date, opts) ([]Itinerary, error)
    // Cheapest destinations from origin within window. Explore mode only.
    Explore(ctx, origin string, window DateRange, nights Range, region string, opts) ([]DestPrice, error)
}

type AwardSource interface {
    // Per-day availability + mileage cost for a route over a window across programs.
    Availability(ctx, origin, dest string, window DateRange, cabin string) ([]AwardDay, error)
    // Destinations with availability from origin (explore).
    Bulk(ctx, program, origin string, window DateRange, cabin string) ([]AwardDay, error)
}
```

## Cash: `google` (default)

Google Flights frontend, Go port of the `tfs` protobuf used by fast-flights / fli. Free, best coverage (OTAs, LCCs, basic economy), has a calendar/date-grid view (one request covers ~2 months for a route) and an Explore RPC (one request returns cheapest destinations from an origin for a window). Unofficial: expect to patch the protobuf a couple of times a year. Run from a residential IP (`tree`), not Hetzner, to avoid bot blocking. Pace: ≤1 request/2s, jittered, realistic UA, `hl=en&curr=USD`.

## Cash: `serpapi` (fallback)

Same Google data via SerpApi, ~$75/mo for 5k calls. Same response shape → same parser downstream. No explore endpoint; explore mode falls back to the curated destination list (below) when this source is active. Toggle in Settings.

## Award: `seatsaero`

Pro API ($9.99/mo, 1,000 calls/day, `X-RateLimit-Remaining` header respected). Cached Search gives per-day availability and mileage per cabin for a route across sources (`delta`, `virginatlantic`, `aeroplan`, `united`, `flyingblue`, ...). Bulk Availability by source + origin covers explore. No Live Search for Pro users, so data is as fresh as seats.aero's crawl (hours). Alerts say "seen Xh ago" and the Book link opens the airline's award search to confirm.

## Curated destinations (explore fallback, and region filter)

`pkg/geo/destinations.go`: ~150 airports tagged by region. Used to (a) filter Explore results by `--region`, (b) fan out `Calendar` calls when the active cash source has no Explore. Not a data source; just a list.

# Scan Algorithm

A scan runs per active trip, per traveler origin, per source. Budget-aware: counts seats.aero remaining calls; Google calls paced. Cheapest-first ordering so fixed trips always complete.

- **fixed**: for each traveler origin × dest: `Search` for each date pair in the slack grid ((2·slack+1)² pairs, usually 1). `Availability` once per route. Store offers.
- **flexible**: for each origin × dest: `Calendar` per month in window (one-way out prices and return prices). Combine into date pairs satisfying the nights range. Rank by Σ travelers. For top 5 pairs, `Search` to get real itineraries (airline, stops, basic-economy flag). `Availability` once per route per window.
- **explore**: for each origin: `Explore` → list of (dest, lowest price in window). Intersect across travelers (dest must be present and ≤ target for every origin; if no target, use the baseline rule). For top 10 intersected destinations, run the flexible algorithm. Award side: `Bulk` per program per origin; intersect the same way.

Each scan writes to `scans` (trip, source, started, finished, calls_used, error). Offers are append-only with `seen_at`; the UI reads the latest scan per trip.

# Alert Rules

Evaluated after each scan, per trip. An alert fires when any of:

1. **Under target**: candidate effective per-person ≤ `target`, and no prior alert for `(trip, dest, out, ret, program)` within 20% of this price. Re-alerts on further ≥10% drops.
2. **New low**: candidate is the lowest effective price seen for this trip and ≥10% below the previous low (fixed/flexible only; prevents silence when target is too optimistic).
3. **Award space opened**: award availability appeared on a route/date that had none last scan and effective ≤ target (award space is binary; it's the event that matters).
4. **Explore deal** (explore mode without target): candidate ≤ 65% of the route's 90-day median lowest price from `price_history`. Cold start: no alerts until 14 days of history for that route; `--target` works immediately.

Explore alerts are batched into one message per scan (top 5 destinations); other modes send one message per candidate. Message text:

```
✈ nrt-mar10 · SFO→NRT Mar 10–20 · $512/pp cash (Delta, nonstop) · was $689
   sam: AUS→NRT $598 cash | 60k VS + $45 ($885 eff)
   https://flights.tail/trips/nrt-mar10
```

Alerts are recorded in `alerts` before sending, so a crashed notify command loses at most one scan's messages rather than re-sending forever. Alert text carries the trip detail URL.

# Web UI

The only surface. Partners use it, so nothing requires a shell. Go `net/http` + `html/template` + htmx for partial updates; no JS build step. Written in Go, not a JS framework: the UI is tables and forms, and one `go build` ships the whole app. Pages:

- **Trips**: table of trips (mode, travelers, best effective price per person and total, Δ since last scan, last scanned, status). Row actions: pause/resume, delete. Add-trip form with three tabs matching the modes:
  - Destination: airport or metro code (`NRT`, `TYO`, `LON`) with typeahead; absent on the Explore tab, which has a region select (`anywhere`, `europe`, `asia`, `latam`, ...).
  - Dates: Fixed tab = depart + return pickers and ±slack days. Flexible/Explore tabs = window start/end, nights min–max, "weekends only" (Fri/Sat out, Sun/Mon back).
  - Travelers: checkboxes, all selected by default.
  - Cabin (economy/premium/business), target effective USD per person (required for fixed/flexible, optional for explore), max stops, airline allow-list, no basic economy, one-way.
  - Trip id is a short slug (`nrt-mar10`) used in URLs and alert text.
- **Trip detail**: ranked candidate table (dest, dates, total, per-traveler breakdown showing cash vs best bookable award and program, "or N MR via Amex Travel" next to cash), sparkline of lowest price per scan, alert history, "Book" links per traveler: Google Flights URL, airline/program award search prefilled with dates (Delta, Virgin Atlantic, Aeroplan, United, Flying Blue, Southwest), Amex Travel. Travelers are marked booked individually; the trip keeps scanning for the rest and stops when everyone is booked.
- **Explore**: deals board for explore-mode trips: destination cards with price per traveler, deal score vs baseline, best date pairs. The Flights-From-Home-style digest, but intersected across everyone's home airport.
- **Travelers**: add/remove; name, home airports (one or more; any is an acceptable origin), points balances per program (`delta`, `southwest`, `united`, `amex` = Membership Rewards) with inline edit and last-updated timestamps.
- **Settings**: seats.aero key (optional; without it award scanning is skipped), SerpApi key + cash source toggle (`google` default / `serpapi`), cpp table, Southwest rate, Amex rebate (airline + pct), notify command + Matrix room. Scan health: last 20 scans with calls used, errors, seats.aero calls remaining today. "Scan now" button.
- **Route history** (`/history/SFO/NRT`): lowest price per day over time; the baseline the explore deal score uses.


# Storage

SQLite, migrations embedded, same pattern as money.

- `travelers(id, name, homes TEXT /* comma list */)`
- `balances(traveler_id, program, miles, updated_at)` — `program` includes `amex`
- `trips(id TEXT PK, mode, dest, region, depart, ret, slack, window_start, window_end, nights_min, nights_max, weekends BOOL, one_way BOOL, cabin, target_usd, max_stops, airlines, no_basic BOOL, status /* active|paused|booked */, created_at)`
- `trip_travelers(trip_id, traveler_id, booked BOOL)`
- `scans(id, trip_id, source, started_at, finished_at, calls_used, error)`
- `offers(id, scan_id, trip_id, traveler_id, origin, dest, out_date, ret_date, kind /* cash|award */, program, price_usd, miles, taxes_usd, airline, stops, basic_economy BOOL, seen_at, link_params JSON)`
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

- Google protobuf drift breaks the cash source: detected by `scans.error` + a daily "scan health" alert when every trip errors twice in a row. Mitigation is the SerpApi flip.
- seats.aero API access is not guaranteed to all Pro accounts and may be geo-restricted; check Settings → API before building on it. Without it the tool is cash-only.
- Award data is cached (hours old). Alerts say so; humans verify on the airline site.
- Multi-origin sync is same-day only. Good enough for v1; tighter arrival windows need per-itinerary times, which `Search` provides and could be added later.

# Later

- Nearby-airport expansion (`--nearby 100mi`).
- Positioning flights (fly cheap to a hub, then award).
- Price-drop tracking on booked trips for Delta/United refundable credits.
