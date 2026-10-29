// Package source defines the data-provider interfaces the scanner drives.
// Implementations: google (Google Flights search page), seatsaero (planned). See design_doc.md "Sources".
package source

import (
	"context"
	"errors"

	"github.com/arjungandhi/flights/pkg/model"
)

// Query is one priced lookup: a route on a date pair for one adult.
type Query struct {
	Origin   string
	Dest     string
	Out      string // YYYY-MM-DD
	Ret      string // empty = one-way
	Cabin    model.Cabin
	MaxStops int // -1 any
	Airlines []string
}

type Itinerary struct {
	PriceUSD float64
	Airline  string // marketing carrier of the first leg
	Stops    int
	Legs     string // "UA837 SFO→NRT, NH7 NRT→..." for display
	Duration int    // minutes
	Depart   string // HH:MM local, first leg
	Arrive   string // HH:MM local, last leg
	ArriveOn string // YYYY-MM-DD the last leg lands; differs from the search date on red-eyes
	Link     string // opaque params to rebuild a booking link; source-specific
}

// Insight is the provider's view of whether this date pair is cheap right now.
type Insight struct {
	Level      int // 1 low, 2 typical, 3 high; 0 unknown
	TypicalLow float64
	TypicalHi  float64
}

type Result struct {
	Itineraries []Itinerary // cheapest first
	Insight     Insight
}

type CashSource interface {
	Name() string
	// Search prices one route on one date pair. Implementations pace themselves.
	Search(ctx context.Context, q Query) (Result, error)
}

type DateRange struct{ Start, End string }

type AwardDay struct {
	Date     string
	Program  string
	Miles    int64
	TaxesUSD float64
	Airline  string
	Stops    int
	SeenAt   string
}

type AwardSource interface {
	Name() string
	Availability(ctx context.Context, origin, dest string, window DateRange, cabin model.Cabin) ([]AwardDay, error)
	Remaining() int
}

var ErrBlocked = errors.New("provider returned a page without results; likely rate limited or layout changed")
