package web

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/arjungandhi/flights/pkg/model"
	"github.com/arjungandhi/flights/pkg/source/google"
)

// bookLink returns where a human goes to book this offer. Google Flights and Southwest/United/Aeroplan
// accept prefilled search parameters; Delta, Virgin Atlantic, Flying Blue and Amex Travel do not expose
// stable deep links, so those land on the search page.
func bookLink(o model.Offer) string {
	rt := o.RetDate != ""
	switch {
	case o.Kind == model.KindCash && o.LinkParams != "":
		return google.LinkURL(o.LinkParams)
	case o.Kind == model.KindCash:
		q := fmt.Sprintf("Flights from %s to %s on %s", o.Origin, o.Dest, o.OutDate)
		if rt {
			q += " through " + o.RetDate
		}
		return "https://www.google.com/travel/flights?" + url.Values{"q": {q}, "curr": {"USD"}}.Encode()
	case o.Program == "southwest":
		v := url.Values{
			"originationAirportCode": {o.Origin}, "destinationAirportCode": {o.Dest},
			"departureDate": {o.OutDate}, "adultPassengersCount": {"1"}, "fareType": {"POINTS"},
			"tripType": {"oneway"},
		}
		if rt {
			v.Set("tripType", "roundtrip")
			v.Set("returnDate", o.RetDate)
		}
		return "https://www.southwest.com/air/booking/select.html?" + v.Encode()
	case o.Program == "united":
		v := url.Values{"f": {o.Origin}, "t": {o.Dest}, "d": {o.OutDate}, "tt": {"2"}, "at": {"1"}, "sc": {"7"}, "px": {"1"}}
		if rt {
			v.Set("tt", "1")
			v.Set("r", o.RetDate)
		}
		return "https://www.united.com/en/us/fsr/choose-flights?" + v.Encode()
	case o.Program == "aeroplan":
		v := url.Values{"org0": {o.Origin}, "dest0": {o.Dest}, "departureDate0": {o.OutDate}, "ADT": {"1"}, "tripType": {"O"}, "marketCode": {"INT"}}
		if rt {
			v.Set("tripType", "R")
			v.Set("org1", o.Dest)
			v.Set("dest1", o.Origin)
			v.Set("departureDate1", o.RetDate)
		}
		return "https://www.aircanada.com/aeroplan/redeem/availability/outbound?" + v.Encode()
	case o.Program == "delta":
		return "https://www.delta.com/flight-search/book-a-flight"
	case o.Program == "virginatlantic":
		return "https://www.virginatlantic.com/flight-search/book-a-flight"
	case o.Program == "flyingblue":
		return "https://www.airfrance.us/search/offers"
	}
	return ""
}

// amexLink opens the same search on Amex Travel (pay with points). The site encodes its
// search as base64(urlencode(JSON)) behind a login redirect; captured 2026-10-28 from a live
// search. Locations are keyed by IATA code as ZAPRT-GEONAMES-<code>.
func amexLink(o model.Offer, cabin model.Cabin) string {
	loc := func(code string) map[string]any {
		return map[string]any{"query": code, "name": code, "label": code, "airportCode": code, "type": "AIRPORT",
			"id": "ZAPRT-GEONAMES-" + code, "searchIdType": "LOCATION_ID"}
	}
	us := func(d string) string {
		t, err := time.Parse(model.DateLayout, d)
		if err != nil {
			return ""
		}
		return t.Format("01/02/2006")
	}
	class, trip := "ECONOMY", "ROUND_TRIP"
	switch cabin {
	case model.CabinPremium:
		class = "PREMIUM_ECONOMY"
	case model.CabinBusiness:
		class = "BUSINESS"
	}
	flight := map[string]any{"startLocation": loc(o.Origin), "endLocation": loc(o.Dest), "startDate": us(o.OutDate), "startTime": "ANYTIME", "endTime": "ANYTIME"}
	if o.RetDate == "" {
		trip = "ONE_WAY"
	} else {
		flight["endDate"] = us(o.RetDate)
	}
	body, _ := json.Marshal(map[string]any{
		"request": map[string]any{
			"flights":     []any{flight},
			"travelers":   []any{map[string]any{"adults": 1, "children": []any{}, "infants": []any{}, "pets": 0}},
			"flightClass": class, "tripType": trip, "isNonStop": false, "isFlexibleDate": false, "airline": nil,
		},
		"searchType": "flights",
	})
	// encodeURIComponent-style: QueryEscape but %20 for spaces.
	rb := base64.StdEncoding.EncodeToString([]byte(strings.ReplaceAll(url.QueryEscape(string(body)), "+", "%20")))
	return "https://prebooking.americanexpress.com/en-us/travel/search-redirect?requestBody=" + url.QueryEscape(rb)
}
