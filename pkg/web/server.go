// Package web serves the UI. Plain forms + a little htmx; templates and CSS are embedded.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/arjungandhi/flights/pkg/db"
	"github.com/arjungandhi/flights/pkg/geo"
	"github.com/arjungandhi/flights/pkg/model"
	"github.com/arjungandhi/flights/pkg/pricing"
	"github.com/arjungandhi/flights/pkg/scan"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/* static/fonts/*
var staticFS embed.FS

type Server struct {
	DB     *db.DB
	Runner *scan.Runner
	Every  time.Duration
	pages  map[string]*template.Template
}

func (s *Server) Handler() http.Handler {
	s.pages = map[string]*template.Template{}
	for _, name := range []string{"trips", "trip", "explore", "travelers", "settings", "history"} {
		s.pages[name] = template.Must(template.New("layout.html").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html"))
	}

	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	mux.HandleFunc("GET /{$}", s.trips)
	mux.HandleFunc("POST /trips", s.addTrip)
	mux.HandleFunc("GET /trips/{id}", s.trip)
	mux.HandleFunc("POST /trips/{id}/status", s.tripStatus)
	mux.HandleFunc("POST /trips/{id}/delete", s.deleteTrip)
	mux.HandleFunc("POST /trips/{id}/booked", s.booked)
	mux.HandleFunc("GET /explore", s.explore)
	mux.HandleFunc("GET /travelers", s.travelers)
	mux.HandleFunc("POST /travelers", s.addTraveler)
	mux.HandleFunc("POST /travelers/{id}", s.updateTraveler)
	mux.HandleFunc("POST /travelers/{id}/delete", s.deleteTraveler)
	mux.HandleFunc("POST /travelers/{id}/balances", s.setBalances)
	mux.HandleFunc("GET /settings", s.settings)
	mux.HandleFunc("POST /settings", s.saveSettings)
	mux.HandleFunc("POST /scan", s.scanNow)
	mux.HandleFunc("GET /history/{origin}/{dest}", s.history)
	return logRequests(mux)
}

func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

type page struct {
	Title  string
	Active string
	Error  string
	Flash  string
	Data   any
}

func (s *Server) render(w http.ResponseWriter, name string, p page) {
	if p.Active == "" {
		p.Active = name
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pages[name].ExecuteTemplate(w, "layout.html", p); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

var funcs = template.FuncMap{
	"usd": func(v float64) string {
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return "—"
		}
		return "$" + commas(int64(math.Round(v)))
	},
	"pts": func(v int64) string {
		if v >= 1000 {
			return fmt.Sprintf("%.0fk", float64(v)/1000)
		}
		return commas(v)
	},
	"n": commas,
	"ago": func(t time.Time) string {
		if t.IsZero() {
			return "never"
		}
		d := time.Since(t)
		switch {
		case d < time.Minute:
			return "just now"
		case d < time.Hour:
			return fmt.Sprintf("%dm ago", int(d.Minutes()))
		case d < 48*time.Hour:
			return fmt.Sprintf("%dh ago", int(d.Hours()))
		default:
			return fmt.Sprintf("%dd ago", int(d.Hours()/24))
		}
	},
	"date": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Local().Format("Jan 2 15:04")
	},
	"handDate": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return strings.ToLower(t.Local().Format("Jan 2 · 15:04"))
	},
	"city": func(code string) string {
		if c := geo.City(code); c != "" {
			return c
		}
		return code
	},
	"shortDate": func(s string) string {
		t, err := time.Parse(model.DateLayout, s)
		if err != nil {
			return s
		}
		return t.Format("Jan 2")
	},
	"program": func(id string) string {
		for _, p := range model.BalancePrograms {
			if p.ID == id {
				return p.Name
			}
		}
		return id
	},
	"amexPts":   pricing.AmexTravelPoints,
	"effective": pricing.Effective,
	"dur": func(d time.Duration) string {
		if d%time.Hour == 0 {
			return fmt.Sprintf("%dh", int(d.Hours()))
		}
		return d.String()
	},
	"sparkline": sparkline,
	"bookLink":  bookLink,
	"googleLink": func(origin, dest, out, ret string) string {
		return bookLink(model.Offer{Kind: model.KindCash, Origin: origin, Dest: dest, OutDate: out, RetDate: ret})
	},
	"has": func(xs []string, x string) bool {
		for _, v := range xs {
			if v == x {
				return true
			}
		}
		return false
	},
	"join": strings.Join,
	"sub":  func(a, b float64) float64 { return a - b },
	"seq": func(n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	},
}

func commas(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := fmt.Sprint(v)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

// sparkline renders points as an inline SVG polyline sized for a table cell.
func sparkline(pts []model.PricePoint) template.HTML {
	if len(pts) < 2 {
		return ""
	}
	const w, h = 160.0, 36.0
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, p := range pts {
		lo, hi = math.Min(lo, p.LowestUSD), math.Max(hi, p.LowestUSD)
	}
	if hi == lo {
		hi = lo + 1
	}
	var b strings.Builder
	for i, p := range pts {
		x := w * float64(i) / float64(len(pts)-1)
		y := h - 2 - (h-4)*(p.LowestUSD-lo)/(hi-lo)
		fmt.Fprintf(&b, "%.1f,%.1f ", x, y)
	}
	return template.HTML(fmt.Sprintf(`<svg class="spark" viewBox="0 0 %.0f %.0f" width="%.0f" height="%.0f"><polyline fill="none" stroke="currentColor" stroke-width="1.5" points="%s"/></svg>`, w, h, w, h, strings.TrimSpace(b.String())))
}
