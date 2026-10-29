// Package geo maps airport codes to city names for display.
package geo

type Destination struct {
	Code string
	City string
}

var all = []Destination{
	{"LAS", "Las Vegas"}, {"LAX", "Los Angeles"}, {"SFO", "San Francisco"},
	{"SAN", "San Diego"}, {"SEA", "Seattle"}, {"PDX", "Portland"},
	{"DEN", "Denver"}, {"PHX", "Phoenix"}, {"AUS", "Austin"},
	{"DFW", "Dallas"}, {"IAH", "Houston"}, {"MSY", "New Orleans"},
	{"ORD", "Chicago"}, {"MSP", "Minneapolis"}, {"DTW", "Detroit"},
	{"BNA", "Nashville"}, {"ATL", "Atlanta"}, {"MIA", "Miami"},
	{"FLL", "Fort Lauderdale"}, {"MCO", "Orlando"}, {"TPA", "Tampa"},
	{"CLT", "Charlotte"}, {"RDU", "Raleigh"}, {"DCA", "Washington"},
	{"PHL", "Philadelphia"}, {"JFK", "New York"}, {"BOS", "Boston"},
	{"SLC", "Salt Lake City"}, {"BZN", "Bozeman"}, {"JAC", "Jackson Hole"},
	{"ANC", "Anchorage"}, {"HNL", "Honolulu"}, {"OGG", "Maui"},
	{"KOA", "Kona"}, {"LIH", "Kauai"}, {"YVR", "Vancouver"},
	{"YYZ", "Toronto"}, {"YUL", "Montreal"}, {"YYC", "Calgary"},
	{"MEX", "Mexico City"}, {"CUN", "Cancun"}, {"SJD", "Los Cabos"}, {"PVR", "Puerto Vallarta"},
	{"OAX", "Oaxaca"}, {"GDL", "Guadalajara"}, {"MID", "Merida"}, {"SJO", "San Jose CR"},
	{"LIR", "Liberia CR"}, {"PTY", "Panama City"}, {"GUA", "Guatemala City"}, {"BZE", "Belize City"},
	{"SJU", "San Juan"}, {"PUJ", "Punta Cana"}, {"MBJ", "Montego Bay"}, {"AUA", "Aruba"},
	{"CUR", "Curacao"}, {"NAS", "Nassau"}, {"HAV", "Havana"}, {"BOG", "Bogota"},
	{"MDE", "Medellin"}, {"CTG", "Cartagena"}, {"LIM", "Lima"}, {"CUZ", "Cusco"},
	{"UIO", "Quito"}, {"GPS", "Galapagos"}, {"SCL", "Santiago"}, {"EZE", "Buenos Aires"},
	{"GIG", "Rio de Janeiro"}, {"GRU", "Sao Paulo"}, {"MVD", "Montevideo"}, {"LPB", "La Paz"},
	{"LHR", "London"}, {"DUB", "Dublin"}, {"EDI", "Edinburgh"}, {"CDG", "Paris"},
	{"AMS", "Amsterdam"}, {"BRU", "Brussels"}, {"FRA", "Frankfurt"}, {"MUC", "Munich"},
	{"BER", "Berlin"}, {"ZRH", "Zurich"}, {"GVA", "Geneva"}, {"VIE", "Vienna"},
	{"PRG", "Prague"}, {"BUD", "Budapest"}, {"WAW", "Warsaw"}, {"KRK", "Krakow"},
	{"CPH", "Copenhagen"}, {"ARN", "Stockholm"}, {"OSL", "Oslo"}, {"HEL", "Helsinki"},
	{"KEF", "Reykjavik"}, {"LIS", "Lisbon"}, {"OPO", "Porto"}, {"MAD", "Madrid"},
	{"BCN", "Barcelona"}, {"SVQ", "Seville"}, {"PMI", "Mallorca"}, {"FCO", "Rome"},
	{"MXP", "Milan"}, {"VCE", "Venice"}, {"FLR", "Florence"}, {"NAP", "Naples"},
	{"ATH", "Athens"}, {"JTR", "Santorini"}, {"SPU", "Split"}, {"DBV", "Dubrovnik"},
	{"IST", "Istanbul"}, {"NCE", "Nice"}, {"LYS", "Lyon"}, {"TLL", "Tallinn"},
	{"NRT", "Tokyo"}, {"KIX", "Osaka"}, {"CTS", "Sapporo"}, {"FUK", "Fukuoka"},
	{"ICN", "Seoul"}, {"TPE", "Taipei"}, {"HKG", "Hong Kong"}, {"PVG", "Shanghai"},
	{"PEK", "Beijing"}, {"SIN", "Singapore"}, {"KUL", "Kuala Lumpur"}, {"BKK", "Bangkok"},
	{"CNX", "Chiang Mai"}, {"HKT", "Phuket"}, {"HAN", "Hanoi"}, {"SGN", "Ho Chi Minh City"},
	{"DAD", "Da Nang"}, {"REP", "Siem Reap"}, {"MNL", "Manila"}, {"CEB", "Cebu"},
	{"DPS", "Bali"}, {"CGK", "Jakarta"}, {"DEL", "Delhi"}, {"BOM", "Mumbai"},
	{"BLR", "Bengaluru"}, {"GOI", "Goa"}, {"CMB", "Colombo"}, {"KTM", "Kathmandu"},
	{"MLE", "Maldives"}, {"ULN", "Ulaanbaatar"},
	{"SYD", "Sydney"}, {"MEL", "Melbourne"}, {"BNE", "Brisbane"}, {"CNS", "Cairns"},
	{"PER", "Perth"}, {"AKL", "Auckland"}, {"ZQN", "Queenstown"}, {"CHC", "Christchurch"},
	{"NAN", "Fiji"}, {"PPT", "Tahiti"}, {"RAR", "Rarotonga"},
	{"CPT", "Cape Town"}, {"JNB", "Johannesburg"}, {"NBO", "Nairobi"}, {"JRO", "Kilimanjaro"},
	{"ZNZ", "Zanzibar"}, {"ADD", "Addis Ababa"}, {"CMN", "Casablanca"}, {"RAK", "Marrakech"},
	{"CAI", "Cairo"}, {"ACC", "Accra"}, {"DSS", "Dakar"}, {"SEZ", "Seychelles"},
	{"MRU", "Mauritius"}, {"WDH", "Windhoek"}, {"LVI", "Livingstone"},
	{"DXB", "Dubai"}, {"AUH", "Abu Dhabi"}, {"DOH", "Doha"}, {"MCT", "Muscat"},
	{"AMM", "Amman"}, {"TLV", "Tel Aviv"}, {"RUH", "Riyadh"}, {"JED", "Jeddah"},
	{"BAH", "Bahrain"},
}

func City(code string) string {
	for _, d := range all {
		if d.Code == code {
			return d.City
		}
	}
	return ""
}
