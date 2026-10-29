package scan

import (
	"testing"
	"time"

	"github.com/arjungandhi/flights/pkg/model"
)

var now = time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)

func TestPairsFixedSlack(t *testing.T) {
	trip := model.Trip{Mode: model.ModeFixed, Depart: "2027-03-10", Return: "2027-03-20", Slack: 1}
	got := Pairs(trip, now)
	if len(got) != 9 {
		t.Fatalf("len = %d, want 9", len(got))
	}
	if got[0] != (Pair{"2027-03-09", "2027-03-19"}) || got[8] != (Pair{"2027-03-11", "2027-03-21"}) {
		t.Errorf("bounds = %v .. %v", got[0], got[8])
	}
}

func TestPairsFixedPastDatesDropped(t *testing.T) {
	trip := model.Trip{Mode: model.ModeFixed, Depart: "2027-01-02", Return: "2027-01-05", Slack: 2}
	for _, p := range Pairs(trip, now) {
		if p.Out < "2027-01-02" {
			t.Errorf("past outbound %v", p)
		}
	}
}

func TestPairsFlexibleWeekends(t *testing.T) {
	trip := model.Trip{Mode: model.ModeFlexible, WindowStart: "2027-05-01", WindowEnd: "2027-05-31", NightsMin: 2, NightsMax: 3, Weekends: true}
	got := Pairs(trip, now)
	if len(got) == 0 {
		t.Fatal("no pairs")
	}
	for _, p := range got {
		o, _ := time.Parse(model.DateLayout, p.Out)
		r, _ := time.Parse(model.DateLayout, p.Ret)
		if o.Weekday() != time.Friday && o.Weekday() != time.Saturday {
			t.Errorf("outbound %s is %s", p.Out, o.Weekday())
		}
		if r.Weekday() != time.Sunday && r.Weekday() != time.Monday {
			t.Errorf("return %s is %s", p.Ret, r.Weekday())
		}
		if n := p.nights(); n < 2 || n > 3 {
			t.Errorf("nights %d for %v", n, p)
		}
	}
}

func TestPairsReturnInsideWindow(t *testing.T) {
	trip := model.Trip{Mode: model.ModeFlexible, WindowStart: "2027-05-01", WindowEnd: "2027-05-10", NightsMin: 7, NightsMax: 12}
	got := Pairs(trip, now)
	for _, p := range got {
		if p.Ret > "2027-05-10" {
			t.Errorf("return past window: %v", p)
		}
	}
	if len(got) != 6 { // out May 1..3 with 7..9 nights fit; May 1: 7,8,9; May 2: 7,8; May 3: 7
		t.Errorf("len = %d, want 6: %v", len(got), got)
	}
}

func TestSampleSpread(t *testing.T) {
	var pairs []Pair
	for i := range 100 {
		pairs = append(pairs, Pair{Out: time.Date(2027, 6, 1+i, 0, 0, 0, 0, time.UTC).Format(model.DateLayout)})
	}
	got := Sample(pairs, 5)
	if len(got) != 5 || got[0] != pairs[0] || got[4] != pairs[99] {
		t.Errorf("sample = %v", got)
	}
	if s := Sample(pairs[:3], 10); len(s) != 3 {
		t.Errorf("small input should pass through, got %d", len(s))
	}
	if s := Sample(pairs, 1); len(s) != 1 || s[0] != pairs[50] {
		t.Errorf("n=1 should return the middle pair, got %v", s)
	}
}

func TestNeighborsSameLength(t *testing.T) {
	trip := model.Trip{Mode: model.ModeFlexible, WindowStart: "2027-05-01", WindowEnd: "2027-05-31", NightsMin: 7, NightsMax: 8}
	pairs := Pairs(trip, now)
	center := Pair{"2027-05-10", "2027-05-17"}
	got := Neighbors(pairs, center, 2, map[Pair]bool{{"2027-05-11", "2027-05-18"}: true})
	want := []Pair{{"2027-05-08", "2027-05-15"}, {"2027-05-09", "2027-05-16"}, {"2027-05-12", "2027-05-19"}}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v want %v", got, want)
		}
	}
}
