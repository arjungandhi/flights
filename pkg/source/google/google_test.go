package google

import (
	"os"
	"testing"

	"github.com/arjungandhi/flights/pkg/model"
	"github.com/arjungandhi/flights/pkg/source"
)

// Captured 2026-10-28 from the live page for SFO→LAS one-way 2027-03-15 (20 rows across the
// "best" and "other" blocks, Southwest included). Layout drift in Google's payload shows up here.
func TestParseFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/sfo-las-ow.json")
	if err != nil {
		t.Fatal(err)
	}
	res, err := parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Itineraries) != 20 {
		t.Fatalf("itineraries = %d, want 20", len(res.Itineraries))
	}
	first := res.Itineraries[0]
	if first.PriceUSD != 79 || first.Airline != "WN" || first.Stops != 0 || first.Legs != "WN2381 SFO→LAS" || first.Duration != 90 {
		t.Errorf("first = %+v", first)
	}
	for i := 1; i < len(res.Itineraries); i++ {
		if res.Itineraries[i].PriceUSD < res.Itineraries[i-1].PriceUSD {
			t.Errorf("not sorted by price at %d", i)
		}
	}
	if res.Insight != (source.Insight{Level: 3, TypicalLow: 25, TypicalHi: 130}) {
		t.Errorf("insight = %+v", res.Insight)
	}
}

func TestEncodeTFS(t *testing.T) {
	q := source.Query{Origin: "SFO", Dest: "NRT", Out: "2027-03-10", Ret: "2027-03-20", Cabin: model.CabinEconomy, MaxStops: -1}
	const want = "CBwQAhoeEgoyMDI3LTAzLTEwagcIARIDU0ZPcgcIARIDTlJUGh4SCjIwMjctMDMtMjBqBwgBEgNOUlRyBwgBEgNTRk9AAUgBcAGYAQE"
	if got := encodeTFS(q); got != want {
		t.Errorf("round trip tfs\n got %s\nwant %s", got, want)
	}
	one := encodeTFS(source.Query{Origin: "SFO", Dest: "LAS", Out: "2027-03-10", MaxStops: 0, Airlines: []string{"WN"}})
	if one == "" || one == want {
		t.Errorf("one-way tfs not distinct")
	}
}

func TestExtractPayloadMissing(t *testing.T) {
	if _, err := extractPayload([]byte(`<html>AF_initDataCallback({key: 'ds:0', data:[1], sideChannel: {}});</html>`)); err == nil {
		t.Error("expected error when ds:1 absent")
	}
}
