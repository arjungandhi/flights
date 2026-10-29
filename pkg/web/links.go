package web

import (
	"fmt"
	"net/url"

	"github.com/arjungandhi/flights/pkg/model"
)

// bookLink returns where a human goes to book this offer. Google Flights and Southwest/United/Aeroplan
// accept prefilled search parameters; Delta, Virgin Atlantic, Flying Blue and Amex Travel do not expose
// stable deep links, so those land on the search page.
func bookLink(o model.Offer) string {
	rt := o.RetDate != ""
	switch {
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

const amexTravelURL = "https://www.amextravel.com/flight-searches"
