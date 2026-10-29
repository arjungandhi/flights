// Package geo is the curated destination list explore mode fans out over, since Google's
// explore endpoint is not reachable without a browser session. Airport codes only; one
// primary airport per city keeps the page budget sane.
package geo

import "sort"

type Destination struct {
	Code   string
	City   string
	Region string
}

var all = []Destination{
	// north-america
	{"LAS", "Las Vegas", "north-america"}, {"LAX", "Los Angeles", "north-america"}, {"SFO", "San Francisco", "north-america"},
	{"SAN", "San Diego", "north-america"}, {"SEA", "Seattle", "north-america"}, {"PDX", "Portland", "north-america"},
	{"DEN", "Denver", "north-america"}, {"PHX", "Phoenix", "north-america"}, {"AUS", "Austin", "north-america"},
	{"DFW", "Dallas", "north-america"}, {"IAH", "Houston", "north-america"}, {"MSY", "New Orleans", "north-america"},
	{"ORD", "Chicago", "north-america"}, {"MSP", "Minneapolis", "north-america"}, {"DTW", "Detroit", "north-america"},
	{"BNA", "Nashville", "north-america"}, {"ATL", "Atlanta", "north-america"}, {"MIA", "Miami", "north-america"},
	{"FLL", "Fort Lauderdale", "north-america"}, {"MCO", "Orlando", "north-america"}, {"TPA", "Tampa", "north-america"},
	{"CLT", "Charlotte", "north-america"}, {"RDU", "Raleigh", "north-america"}, {"DCA", "Washington", "north-america"},
	{"PHL", "Philadelphia", "north-america"}, {"JFK", "New York", "north-america"}, {"BOS", "Boston", "north-america"},
	{"SLC", "Salt Lake City", "north-america"}, {"BZN", "Bozeman", "north-america"}, {"JAC", "Jackson Hole", "north-america"},
	{"ANC", "Anchorage", "north-america"}, {"HNL", "Honolulu", "north-america"}, {"OGG", "Maui", "north-america"},
	{"KOA", "Kona", "north-america"}, {"LIH", "Kauai", "north-america"}, {"YVR", "Vancouver", "north-america"},
	{"YYZ", "Toronto", "north-america"}, {"YUL", "Montreal", "north-america"}, {"YYC", "Calgary", "north-america"},
	// latam
	{"MEX", "Mexico City", "latam"}, {"CUN", "Cancun", "latam"}, {"SJD", "Los Cabos", "latam"}, {"PVR", "Puerto Vallarta", "latam"},
	{"OAX", "Oaxaca", "latam"}, {"GDL", "Guadalajara", "latam"}, {"MID", "Merida", "latam"}, {"SJO", "San Jose CR", "latam"},
	{"LIR", "Liberia CR", "latam"}, {"PTY", "Panama City", "latam"}, {"GUA", "Guatemala City", "latam"}, {"BZE", "Belize City", "latam"},
	{"SJU", "San Juan", "latam"}, {"PUJ", "Punta Cana", "latam"}, {"MBJ", "Montego Bay", "latam"}, {"AUA", "Aruba", "latam"},
	{"CUR", "Curacao", "latam"}, {"NAS", "Nassau", "latam"}, {"HAV", "Havana", "latam"}, {"BOG", "Bogota", "latam"},
	{"MDE", "Medellin", "latam"}, {"CTG", "Cartagena", "latam"}, {"LIM", "Lima", "latam"}, {"CUZ", "Cusco", "latam"},
	{"UIO", "Quito", "latam"}, {"GPS", "Galapagos", "latam"}, {"SCL", "Santiago", "latam"}, {"EZE", "Buenos Aires", "latam"},
	{"GIG", "Rio de Janeiro", "latam"}, {"GRU", "Sao Paulo", "latam"}, {"MVD", "Montevideo", "latam"}, {"LPB", "La Paz", "latam"},
	// europe
	{"LHR", "London", "europe"}, {"DUB", "Dublin", "europe"}, {"EDI", "Edinburgh", "europe"}, {"CDG", "Paris", "europe"},
	{"AMS", "Amsterdam", "europe"}, {"BRU", "Brussels", "europe"}, {"FRA", "Frankfurt", "europe"}, {"MUC", "Munich", "europe"},
	{"BER", "Berlin", "europe"}, {"ZRH", "Zurich", "europe"}, {"GVA", "Geneva", "europe"}, {"VIE", "Vienna", "europe"},
	{"PRG", "Prague", "europe"}, {"BUD", "Budapest", "europe"}, {"WAW", "Warsaw", "europe"}, {"KRK", "Krakow", "europe"},
	{"CPH", "Copenhagen", "europe"}, {"ARN", "Stockholm", "europe"}, {"OSL", "Oslo", "europe"}, {"HEL", "Helsinki", "europe"},
	{"KEF", "Reykjavik", "europe"}, {"LIS", "Lisbon", "europe"}, {"OPO", "Porto", "europe"}, {"MAD", "Madrid", "europe"},
	{"BCN", "Barcelona", "europe"}, {"SVQ", "Seville", "europe"}, {"PMI", "Mallorca", "europe"}, {"FCO", "Rome", "europe"},
	{"MXP", "Milan", "europe"}, {"VCE", "Venice", "europe"}, {"FLR", "Florence", "europe"}, {"NAP", "Naples", "europe"},
	{"ATH", "Athens", "europe"}, {"JTR", "Santorini", "europe"}, {"SPU", "Split", "europe"}, {"DBV", "Dubrovnik", "europe"},
	{"IST", "Istanbul", "europe"}, {"NCE", "Nice", "europe"}, {"LYS", "Lyon", "europe"}, {"TLL", "Tallinn", "europe"},
	// asia
	{"NRT", "Tokyo", "asia"}, {"KIX", "Osaka", "asia"}, {"CTS", "Sapporo", "asia"}, {"FUK", "Fukuoka", "asia"},
	{"ICN", "Seoul", "asia"}, {"TPE", "Taipei", "asia"}, {"HKG", "Hong Kong", "asia"}, {"PVG", "Shanghai", "asia"},
	{"PEK", "Beijing", "asia"}, {"SIN", "Singapore", "asia"}, {"KUL", "Kuala Lumpur", "asia"}, {"BKK", "Bangkok", "asia"},
	{"CNX", "Chiang Mai", "asia"}, {"HKT", "Phuket", "asia"}, {"HAN", "Hanoi", "asia"}, {"SGN", "Ho Chi Minh City", "asia"},
	{"DAD", "Da Nang", "asia"}, {"REP", "Siem Reap", "asia"}, {"MNL", "Manila", "asia"}, {"CEB", "Cebu", "asia"},
	{"DPS", "Bali", "asia"}, {"CGK", "Jakarta", "asia"}, {"DEL", "Delhi", "asia"}, {"BOM", "Mumbai", "asia"},
	{"BLR", "Bengaluru", "asia"}, {"GOI", "Goa", "asia"}, {"CMB", "Colombo", "asia"}, {"KTM", "Kathmandu", "asia"},
	{"MLE", "Maldives", "asia"}, {"ULN", "Ulaanbaatar", "asia"},
	// oceania
	{"SYD", "Sydney", "oceania"}, {"MEL", "Melbourne", "oceania"}, {"BNE", "Brisbane", "oceania"}, {"CNS", "Cairns", "oceania"},
	{"PER", "Perth", "oceania"}, {"AKL", "Auckland", "oceania"}, {"ZQN", "Queenstown", "oceania"}, {"CHC", "Christchurch", "oceania"},
	{"NAN", "Fiji", "oceania"}, {"PPT", "Tahiti", "oceania"}, {"RAR", "Rarotonga", "oceania"},
	// africa
	{"CPT", "Cape Town", "africa"}, {"JNB", "Johannesburg", "africa"}, {"NBO", "Nairobi", "africa"}, {"JRO", "Kilimanjaro", "africa"},
	{"ZNZ", "Zanzibar", "africa"}, {"ADD", "Addis Ababa", "africa"}, {"CMN", "Casablanca", "africa"}, {"RAK", "Marrakech", "africa"},
	{"CAI", "Cairo", "africa"}, {"ACC", "Accra", "africa"}, {"DSS", "Dakar", "africa"}, {"SEZ", "Seychelles", "africa"},
	{"MRU", "Mauritius", "africa"}, {"WDH", "Windhoek", "africa"}, {"LVI", "Livingstone", "africa"},
	// middle-east
	{"DXB", "Dubai", "middle-east"}, {"AUH", "Abu Dhabi", "middle-east"}, {"DOH", "Doha", "middle-east"}, {"MCT", "Muscat", "middle-east"},
	{"AMM", "Amman", "middle-east"}, {"TLV", "Tel Aviv", "middle-east"}, {"RUH", "Riyadh", "middle-east"}, {"JED", "Jeddah", "middle-east"},
	{"BAH", "Bahrain", "middle-east"},
}

// Destinations returns the list for a region ("anywhere" or "" = all), sorted by code,
// with the given airport codes removed (nobody explores to their own home airport).
func Destinations(region string, exclude []string) []Destination {
	skip := map[string]bool{}
	for _, c := range exclude {
		skip[c] = true
	}
	var out []Destination
	for _, d := range all {
		if skip[d.Code] || (region != "" && region != "anywhere" && d.Region != region) {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func City(code string) string {
	for _, d := range all {
		if d.Code == code {
			return d.City
		}
	}
	return ""
}
