package google

import (
	"encoding/base64"
	"strings"

	"github.com/arjungandhi/flights/pkg/model"
	"github.com/arjungandhi/flights/pkg/source"
)

// tfs is the protobuf the search page is addressed by. Field layout (reverse-engineered,
// cross-checked against github.com/punitarani/fli):
//
//	1  = 28                      8  = passenger kind, repeated (1 adult)
//	2  = 2                       9  = cabin (1 economy, 2 premium, 3 business, 4 first)
//	3  = segment, repeated       14 = 1
//	3.2  = date                  19 = 2 one-way, 1 round-trip
//	3.5  = max stops (0 nonstop); omit for any
//	3.6  = carrier include, repeated
//	3.13 = origin {1:1, 2:code}  3.14 = destination {1:1, 2:code}
func encodeTFS(q source.Query) string {
	seg := segment(q.Origin, q.Dest, q.Out, q.MaxStops, q.Airlines)
	oneWay := q.Ret == ""
	if !oneWay {
		seg = append(seg, segment(q.Dest, q.Origin, q.Ret, q.MaxStops, q.Airlines)...)
	}
	p := append(varintField(1, 28), varintField(2, 2)...)
	p = append(p, seg...)
	p = append(p, varintField(8, 1)...)
	p = append(p, varintField(9, cabinCode(q.Cabin))...)
	p = append(p, varintField(14, 1)...)
	if oneWay {
		p = append(p, varintField(19, 2)...)
	} else {
		p = append(p, varintField(19, 1)...)
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(p), "=")
}

func segment(origin, dest, date string, maxStops int, airlines []string) []byte {
	body := bytesField(2, []byte(date))
	if maxStops >= 0 {
		body = append(body, varintField(5, uint64(maxStops))...)
	}
	for _, a := range airlines {
		body = append(body, bytesField(6, []byte(a))...)
	}
	body = append(body, bytesField(13, append(varintField(1, 1), bytesField(2, []byte(origin))...))...)
	body = append(body, bytesField(14, append(varintField(1, 1), bytesField(2, []byte(dest))...))...)
	return bytesField(3, body)
}

func cabinCode(c model.Cabin) uint64 {
	switch c {
	case model.CabinPremium:
		return 2
	case model.CabinBusiness:
		return 3
	default:
		return 1
	}
}

func varint(v uint64) []byte {
	var b []byte
	for {
		c := byte(v & 0x7f)
		v >>= 7
		if v == 0 {
			return append(b, c)
		}
		b = append(b, c|0x80)
	}
}

func varintField(field, v uint64) []byte {
	return append(varint(field<<3), varint(v)...)
}

func bytesField(field uint64, p []byte) []byte {
	out := append(varint(field<<3|2), varint(uint64(len(p)))...)
	return append(out, p...)
}
