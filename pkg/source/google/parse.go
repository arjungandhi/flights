package google

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/arjungandhi/flights/pkg/source"
)

// Payload positions (ds:1), from live captures:
//
//	[2][0] best itineraries, [3][0] other itineraries — same row shape
//	row[0] = detail: [0] primary airline code, [2] legs, [9] duration minutes
//	row[1] = price: [[..., amount], token]; empty head = no pre-computed price
//	leg[22] = [airline, flight number, operating airline], leg[3] from, leg[6] to, leg[11] minutes
//	[5] = price insight: [level, [_,current], [_,usual], [_,delta], [_,typical low], [_,typical high], ...]
func parse(raw json.RawMessage) (source.Result, error) {
	var top []json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return source.Result{}, fmt.Errorf("ds:1 is not an array: %w", err)
	}
	var res source.Result
	for _, idx := range []int{2, 3} {
		if idx >= len(top) {
			continue
		}
		var block []json.RawMessage
		if json.Unmarshal(top[idx], &block) != nil || len(block) == 0 {
			continue
		}
		var rows []json.RawMessage
		if json.Unmarshal(block[0], &rows) != nil {
			continue
		}
		for _, r := range rows {
			if it, ok := parseRow(r); ok {
				res.Itineraries = append(res.Itineraries, it)
			}
		}
	}
	sort.SliceStable(res.Itineraries, func(i, j int) bool { return res.Itineraries[i].PriceUSD < res.Itineraries[j].PriceUSD })
	if len(top) > 5 {
		res.Insight = parseInsight(top[5])
	}
	return res, nil
}

func parseRow(raw json.RawMessage) (source.Itinerary, bool) {
	var row []json.RawMessage
	if json.Unmarshal(raw, &row) != nil || len(row) < 2 {
		return source.Itinerary{}, false
	}
	var detail []json.RawMessage
	if json.Unmarshal(row[0], &detail) != nil || len(detail) < 10 {
		return source.Itinerary{}, false
	}
	var price []json.RawMessage
	if json.Unmarshal(row[1], &price) != nil || len(price) == 0 {
		return source.Itinerary{}, false
	}
	var head []json.RawMessage
	if json.Unmarshal(price[0], &head) != nil || len(head) == 0 {
		return source.Itinerary{}, false // no shopping-list price for this row
	}
	var amount float64
	if json.Unmarshal(head[len(head)-1], &amount) != nil || amount <= 0 {
		return source.Itinerary{}, false
	}
	var legs []json.RawMessage
	if json.Unmarshal(detail[2], &legs) != nil || len(legs) == 0 {
		return source.Itinerary{}, false
	}
	it := source.Itinerary{PriceUSD: amount, Stops: len(legs) - 1}
	json.Unmarshal(detail[9], &it.Duration)
	var parts []string
	for i, l := range legs {
		var leg []json.RawMessage
		if json.Unmarshal(l, &leg) != nil || len(leg) < 23 {
			continue
		}
		var carrier []json.RawMessage
		var airline, num, from, to string
		if json.Unmarshal(leg[22], &carrier) == nil && len(carrier) >= 2 {
			json.Unmarshal(carrier[0], &airline)
			json.Unmarshal(carrier[1], &num)
		}
		json.Unmarshal(leg[3], &from)
		json.Unmarshal(leg[6], &to)
		if i == 0 {
			it.Airline = airline
		}
		parts = append(parts, airline+num+" "+from+"→"+to)
	}
	if it.Airline == "" {
		json.Unmarshal(detail[0], &it.Airline)
	}
	it.Legs = strings.Join(parts, ", ")
	return it, true
}

func parseInsight(raw json.RawMessage) source.Insight {
	var block []json.RawMessage
	if json.Unmarshal(raw, &block) != nil || len(block) < 6 {
		return source.Insight{}
	}
	var in source.Insight
	json.Unmarshal(block[0], &in.Level)
	in.TypicalLow = second(block[4])
	in.TypicalHi = second(block[5])
	return in
}

func second(raw json.RawMessage) float64 {
	var pair []json.RawMessage
	if json.Unmarshal(raw, &pair) != nil || len(pair) < 2 {
		return 0
	}
	var v float64
	json.Unmarshal(pair[1], &v)
	return v
}
