// Package pricing puts cash and award offers on one axis (effective USD per person)
// and folds per-traveler offers into trip candidates.
package pricing

import (
	"math"
	"sort"

	"github.com/arjungandhi/flights/pkg/db"
	"github.com/arjungandhi/flights/pkg/model"
)

// Effective is the USD-equivalent cost of an offer for one person.
func Effective(o model.Offer, s db.Settings) float64 {
	if o.Kind == model.KindCash {
		return o.PriceUSD
	}
	cpp := s.CPP[o.Program]
	if cpp == 0 {
		if p, ok := model.ProgramByID(o.Program); ok {
			cpp = p.DefaultCPP
		}
	}
	return float64(o.Miles)*cpp/100 + o.TaxesUSD
}

// AmexTravelPoints is how many MR points a cash fare costs through Amex Travel with the rebate.
func AmexTravelPoints(cashUSD float64, s db.Settings) int64 {
	return int64(math.Ceil(cashUSD * 100 / s.AmexCPP()))
}

// Bookable reports whether the traveler holds enough points: the program balance, plus MR if transferable.
func Bookable(o model.Offer, t model.Traveler) bool {
	if o.Kind == model.KindCash {
		return true
	}
	have := t.Balances[o.Program].Miles
	if p, ok := model.ProgramByID(o.Program); ok && p.MRTransfer {
		have += t.Balances[model.ProgramAmex].Miles
	}
	return have >= o.Miles
}

// Pick is one traveler's chosen option for a date pair.
type Pick struct {
	Traveler  model.Traveler
	Cash      *model.Offer // chosen cash fare: the cheapest, or a preferred-airline fare within tolerance
	Cheapest  *model.Offer // the cheapest cash fare when Cash is a preferred-airline upgrade; else nil
	Preferred *model.Offer // cheapest fare on a preferred airline, may be nil
	Award     *model.Offer // best bookable award, may be nil
	Effective float64      // min of the two
	UsesAward bool
}

// Candidate is a (dest, out, ret) that every unbooked traveler has an offer for.
type Candidate struct {
	Dest    string
	OutDate string
	RetDate string
	Picks   []Pick
	Total   float64
}

func (c Candidate) PerPerson() float64 {
	if len(c.Picks) == 0 {
		return 0
	}
	return c.Total / float64(len(c.Picks))
}

type dateKey struct{ dest, out, ret string }

// Candidates groups offers by date pair, picks each traveler's cheapest bookable option,
// and returns candidates covering all unbooked travelers, cheapest total first.
func Candidates(trip model.Trip, offers []model.Offer, s db.Settings) []Candidate {
	travelers := trip.Unbooked()
	if len(travelers) == 0 {
		return nil
	}
	byTraveler := map[int64]model.Traveler{}
	for _, t := range travelers {
		byTraveler[t.ID] = t.Traveler
	}
	groups := map[dateKey]map[int64]*Pick{}
	for i := range offers {
		o := &offers[i]
		t, ok := byTraveler[o.TravelerID]
		if !ok {
			continue
		}
		k := dateKey{o.Dest, o.OutDate, o.RetDate}
		g := groups[k]
		if g == nil {
			g = map[int64]*Pick{}
			groups[k] = g
		}
		p := g[o.TravelerID]
		if p == nil {
			p = &Pick{Traveler: t}
			g[o.TravelerID] = p
		}
		eff := Effective(*o, s)
		switch o.Kind {
		case model.KindCash:
			if p.Cash == nil || eff < Effective(*p.Cash, s) {
				p.Cash = o
			}
			if t.Prefers(o.Airline) && (p.Preferred == nil || eff < Effective(*p.Preferred, s)) {
				p.Preferred = o
			}
		case model.KindAward:
			if !Bookable(*o, t) {
				continue
			}
			if p.Award == nil || eff < Effective(*p.Award, s) {
				p.Award = o
			}
		}
	}
	var out []Candidate
	for k, g := range groups {
		if len(g) != len(travelers) {
			continue
		}
		c := Candidate{Dest: k.dest, OutDate: k.out, RetDate: k.ret}
		for _, t := range travelers {
			p := g[t.ID]
			p.Effective = math.Inf(1)
			if p.Cash != nil {
				if p.Preferred != nil && p.Preferred != p.Cash && p.Preferred.PriceUSD <= p.Cash.PriceUSD*(1+s.PreferPct/100) {
					p.Cheapest, p.Cash = p.Cash, p.Preferred
				}
				p.Effective = Effective(*p.Cash, s)
			}
			if p.Award != nil {
				if e := Effective(*p.Award, s); e < p.Effective {
					p.Effective, p.UsesAward = e, true
				}
			}
			c.Picks = append(c.Picks, *p)
			c.Total += p.Effective
		}
		if math.IsInf(c.Total, 1) {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total < out[j].Total
		}
		return out[i].OutDate < out[j].OutDate
	})
	return out
}
