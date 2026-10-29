// Package model holds the domain types shared by storage, pricing, scanning and the web UI.
package model

import (
	"strconv"
	"strings"
	"time"
)

const DateLayout = "2006-01-02"

// Mode is derived from which trip fields are set; see design_doc.md "Trip Modes".
type Mode string

const (
	ModeFixed    Mode = "fixed"
	ModeFlexible Mode = "flexible"
	ModeExplore  Mode = "explore"
)

type Status string

const (
	StatusActive Status = "active"
	StatusPaused Status = "paused"
	StatusBooked Status = "booked"
)

type Cabin string

const (
	CabinEconomy  Cabin = "economy"
	CabinPremium  Cabin = "premium"
	CabinBusiness Cabin = "business"
)

var Cabins = []Cabin{CabinEconomy, CabinPremium, CabinBusiness}

// Region filters explore mode. "anywhere" means no filter.
var Regions = []string{"anywhere", "north-america", "latam", "europe", "asia", "oceania", "africa", "middle-east"}

// Program is a loyalty program an award can be priced in.
type Program struct {
	ID         string
	Name       string
	MRTransfer bool    // Amex Membership Rewards transfers 1:1
	DefaultCPP float64 // cents per point valuation
	Source     string  // where award pricing comes from
}

const ProgramAmex = "amex"

var Programs = []Program{
	{ID: "delta", Name: "Delta SkyMiles", MRTransfer: true, DefaultCPP: 1.2, Source: "seatsaero"},
	{ID: "virginatlantic", Name: "Virgin Atlantic Flying Club", MRTransfer: true, DefaultCPP: 1.4, Source: "seatsaero"},
	{ID: "aeroplan", Name: "Air Canada Aeroplan", MRTransfer: true, DefaultCPP: 1.5, Source: "seatsaero"},
	{ID: "flyingblue", Name: "Flying Blue", MRTransfer: true, DefaultCPP: 1.3, Source: "seatsaero"},
	{ID: "united", Name: "United MileagePlus", MRTransfer: false, DefaultCPP: 1.2, Source: "seatsaero"},
	{ID: "southwest", Name: "Southwest Rapid Rewards", MRTransfer: false, DefaultCPP: 1.4, Source: "derived"},
}

// BalancePrograms are the programs a traveler can hold a balance in. Amex is included
// because MR points are the transferable pool, not a bookable program.
var BalancePrograms = append([]Program{{ID: ProgramAmex, Name: "Amex Membership Rewards"}}, Programs...)

func ProgramByID(id string) (Program, bool) {
	for _, p := range Programs {
		if p.ID == id {
			return p, true
		}
	}
	return Program{}, false
}

type Balance struct {
	Miles     int64
	UpdatedAt time.Time
}

type Traveler struct {
	ID       int64
	Name     string
	Homes    []string // airport codes; any is an acceptable origin
	Balances map[string]Balance
}

func (t Traveler) HomesString() string { return strings.Join(t.Homes, ", ") }

type TripTraveler struct {
	Traveler
	Booked bool
}

type Trip struct {
	ID          string // slug used in URLs and alert text
	Mode        Mode
	Dest        string // airport or metro code; empty for explore
	Region      string // explore only
	Depart      string // fixed: YYYY-MM-DD
	Return      string // fixed: YYYY-MM-DD; empty when one-way
	Slack       int    // fixed: ±days
	WindowStart string // flexible/explore
	WindowEnd   string
	NightsMin   int
	NightsMax   int
	Weekends    bool
	OneWay      bool
	Cabin       Cabin
	TargetUSD   float64 // per person; 0 = none
	MaxStops    int     // -1 = any
	Airlines    []string
	NoBasic     bool
	Status      Status
	CreatedAt   time.Time
	Travelers   []TripTraveler
}

// Where is the human-readable destination: code or region.
func (t Trip) Where() string {
	if t.Mode == ModeExplore {
		if t.Region == "" || t.Region == "anywhere" {
			return "anywhere"
		}
		return t.Region
	}
	return t.Dest
}

// When is the human-readable date spec.
func (t Trip) When() string {
	switch t.Mode {
	case ModeFixed:
		s := t.Depart
		if t.Return != "" {
			s += " → " + t.Return
		}
		if t.Slack > 0 {
			s += " ±" + strconv.Itoa(t.Slack) + "d"
		}
		return s
	default:
		s := t.WindowStart + " → " + t.WindowEnd
		if !t.OneWay {
			s += ", " + strconv.Itoa(t.NightsMin) + "–" + strconv.Itoa(t.NightsMax) + " nights"
		}
		if t.Weekends {
			s += ", weekends"
		}
		return s
	}
}

func (t Trip) Unbooked() []TripTraveler {
	var out []TripTraveler
	for _, tt := range t.Travelers {
		if !tt.Booked {
			out = append(out, tt)
		}
	}
	return out
}

type OfferKind string

const (
	KindCash  OfferKind = "cash"
	KindAward OfferKind = "award"
)

// Offer is one priced option for one traveler on one date pair, from one scan.
type Offer struct {
	ID           int64
	ScanID       int64
	TripID       string
	TravelerID   int64
	Origin       string
	Dest         string
	OutDate      string
	RetDate      string
	Kind         OfferKind
	Program      string // award only
	PriceUSD     float64
	Miles        int64
	TaxesUSD     float64
	Airline      string
	Stops        int
	BasicEconomy bool
	SeenAt       time.Time
	LinkParams   string // source-specific JSON used to build booking links
}

type Scan struct {
	ID         int64
	TripID     string
	Source     string
	StartedAt  time.Time
	FinishedAt time.Time
	CallsUsed  int
	Error      string
}

type Alert struct {
	ID           int64
	TripID       string
	Dest         string
	OutDate      string
	RetDate      string
	Program      string
	EffectiveUSD float64
	SentAt       time.Time
}

type PricePoint struct {
	Date      string
	LowestUSD float64
}
