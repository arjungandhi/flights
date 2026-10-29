package scan

import (
	"time"

	"github.com/arjungandhi/flights/pkg/model"
)

// Pair is one searchable (out, ret) date combination. Ret is empty for one-way trips.
type Pair struct{ Out, Ret string }

func (p Pair) nights() int {
	if p.Ret == "" {
		return 0
	}
	o, _ := time.Parse(model.DateLayout, p.Out)
	r, _ := time.Parse(model.DateLayout, p.Ret)
	return int(r.Sub(o).Hours() / 24)
}

// Pairs enumerates every date combination a trip allows, earliest first, skipping dates
// before tomorrow. Fixed trips expand their ± slack; flexible/explore walk the window.
func Pairs(t model.Trip, now time.Time) []Pair {
	tomorrow := now.AddDate(0, 0, 1).Format(model.DateLayout)
	var out []Pair
	add := func(o, r time.Time, oneWay bool) {
		os := o.Format(model.DateLayout)
		if os < tomorrow {
			return
		}
		if oneWay {
			out = append(out, Pair{Out: os})
			return
		}
		if !r.After(o) {
			return
		}
		out = append(out, Pair{Out: os, Ret: r.Format(model.DateLayout)})
	}
	switch t.Mode {
	case model.ModeFixed:
		dep, err := time.Parse(model.DateLayout, t.Depart)
		if err != nil {
			return nil
		}
		ret, _ := time.Parse(model.DateLayout, t.Return)
		first := -t.Slack
		if t.ArriveBy != "" && first > -1 {
			first = -1 // red-eyes the night before land on the target morning
		}
		for do := first; do <= t.Slack; do++ {
			if t.OneWay {
				add(dep.AddDate(0, 0, do), time.Time{}, true)
				continue
			}
			for dr := -t.Slack; dr <= t.Slack; dr++ {
				add(dep.AddDate(0, 0, do), ret.AddDate(0, 0, dr), false)
			}
		}
	default:
		start, err := time.Parse(model.DateLayout, t.WindowStart)
		if err != nil {
			return nil
		}
		end, err := time.Parse(model.DateLayout, t.WindowEnd)
		if err != nil {
			return nil
		}
		for o := start; !o.After(end); o = o.AddDate(0, 0, 1) {
			if t.Weekends && o.Weekday() != time.Friday && o.Weekday() != time.Saturday {
				continue
			}
			if t.OneWay {
				add(o, time.Time{}, true)
				continue
			}
			for n := t.NightsMin; n <= t.NightsMax; n++ {
				r := o.AddDate(0, 0, n)
				if r.After(end) {
					break
				}
				if t.Weekends && r.Weekday() != time.Sunday && r.Weekday() != time.Monday {
					continue
				}
				add(o, r, false)
			}
		}
	}
	return out
}

// Sample picks up to n pairs spread evenly across the list, always including the first and last.
func Sample(pairs []Pair, n int) []Pair {
	if n <= 0 || len(pairs) == 0 {
		return nil
	}
	if len(pairs) <= n {
		return append([]Pair(nil), pairs...)
	}
	out := make([]Pair, 0, n)
	seen := map[Pair]bool{}
	for i := range n {
		idx := i * (len(pairs) - 1) / (n - 1)
		if n == 1 {
			idx = 0
		}
		if !seen[pairs[idx]] {
			seen[pairs[idx]] = true
			out = append(out, pairs[idx])
		}
	}
	return out
}

// Neighbors returns pairs within ±days of p's outbound date with the same trip length,
// excluding anything in skip. Used to refine around the cheapest coarse samples.
func Neighbors(pairs []Pair, p Pair, days int, skip map[Pair]bool) []Pair {
	po, _ := time.Parse(model.DateLayout, p.Out)
	n := p.nights()
	var out []Pair
	for _, q := range pairs {
		if q == p || skip[q] || q.nights() != n {
			continue
		}
		qo, _ := time.Parse(model.DateLayout, q.Out)
		d := qo.Sub(po).Hours() / 24
		if d < 0 {
			d = -d
		}
		if d <= float64(days) {
			out = append(out, q)
		}
	}
	return out
}
