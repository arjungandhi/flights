// Package source defines the data-provider interfaces the scanner drives.
// Implementations: google (Google Flights frontend), serpapi, seatsaero. See design_doc.md "Sources".
package source

import (
	"context"

	"github.com/arjungandhi/flights/pkg/model"
)

type DateRange struct{ Start, End string } // inclusive, YYYY-MM-DD

type IntRange struct{ Min, Max int }

type Options struct {
	Cabin    model.Cabin
	MaxStops int // -1 any
	Airlines []string
	OneWay   bool
}

// DatePrice is the lowest fare departing on Date (one-way leg).
type DatePrice struct {
	Date     string
	PriceUSD float64
}

type Itinerary struct {
	PriceUSD     float64
	Airline      string
	Stops        int
	BasicEconomy bool
	LinkParams   string
}

// DestPrice is the lowest round-trip price to Dest within an explore window.
type DestPrice struct {
	Dest     string
	OutDate  string
	RetDate  string
	PriceUSD float64
}

type AwardDay struct {
	Date     string
	Program  string
	Miles    int64
	TaxesUSD float64
	Airline  string
	Stops    int
	SeenAt   string // RFC3339; award caches are hours old
}

type CashSource interface {
	Name() string
	// Calendar: lowest price per departure date for a route over a window. One call per (origin, dest, month) ideally.
	Calendar(ctx context.Context, origin, dest string, window DateRange, opts Options) ([]DatePrice, error)
	// Search: itineraries for a specific date pair; only for shortlisted candidates.
	Search(ctx context.Context, origin, dest, out, ret string, opts Options) ([]Itinerary, error)
	// Explore: cheapest destinations from origin within window. Return ErrUnsupported if the source can't.
	Explore(ctx context.Context, origin string, window DateRange, nights IntRange, region string, opts Options) ([]DestPrice, error)
}

type AwardSource interface {
	Name() string
	// Availability: per-day availability and mileage for a route over a window across programs.
	Availability(ctx context.Context, origin, dest string, window DateRange, cabin model.Cabin) ([]AwardDay, error)
	// Bulk: everything available from origin for a program in a window (explore).
	Bulk(ctx context.Context, program, origin string, window DateRange, cabin model.Cabin) ([]AwardDay, error)
	// Remaining daily calls, or -1 if unknown.
	Remaining() int
}

type unsupported struct{}

func (unsupported) Error() string { return "operation not supported by this source" }

var ErrUnsupported error = unsupported{}
