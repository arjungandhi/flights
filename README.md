<p align="center"><img src="pkg/web/static/logo.png" width="140" alt="monkey with a wrench"></p>

# flights

Finds cheap flights, cash or points, for a group of people who may live in different cities. You tell it where/when (or just when), it scans on a schedule, and pings you through [`messages`](https://github.com/arjungandhi/messages) when something is worth booking.

Three ways to ask:

| Mode | You know | Example |
|---|---|---|
| fixed | where and when | NRT, Mar 10–20 |
| flexible | where, not when | LIS, any 7–12 nights May–Aug |
| explore | when, not where | somewhere in Latin America, 4–6 nights, any weekend in Oct |

Every traveler has their own home airports and points balances (Amex MR, Delta, United, Southwest, …). A candidate only counts if everyone can get there on the same dates; the total is the sum of each person's cheapest option, cash or award, in effective dollars.

Design: [design_doc.md](design_doc.md).

## Run

```bash
flights                       # web UI on :8080 + scan every 4h, data in ~/.local/share/flights
flights --notify "messages send"   # pipe alerts to Matrix
flights --demo --no-scan      # seed sample data, UI only
```

Flags: `--addr`, `--every`, `--notify`, `--dir` (or `$FLIGHTS_DIR`), `--demo`, `--no-scan`.

Everything else, including API keys, lives in the UI under Settings.

## How it finds prices

Cash fares come from the public Google Flights search page: one HTTP GET per route and date pair, parsed from the payload the page inlines. Free, no key, includes Southwest, and carries Google's own "typical price" range for the dates. Google gated its JSON APIs in 2026, so this is the only free path; each page is ~2–3 MB and the scanner paces itself at one every ~3 s. Fixed-date trips rescan every cycle; flexible and explore trips sample the date window and rescan daily.

## Status

Working: storage, three trip modes, Google Flights cash pricing, candidate ranking across travelers, alert rules, Matrix notify via `messages`, the whole UI. Not yet: award availability (seats.aero), so points are only compared via Amex Travel pay-with-points and Southwest's revenue-based pricing.

## Install

```bash
go install github.com/arjungandhi/flights/cmd/flights@latest
```

Or with Nix:
```bash
nix build github:arjungandhi/flights
```

## Development

```bash
just build    # go build ./cmd/flights
just demo     # run against a throwaway DB with sample data
just test

git config core.hooksPath .githooks   # keeps vendorHash in flake.nix current
```
