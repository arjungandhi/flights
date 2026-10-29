// flights serve: web UI + scan loop in one process. See design_doc.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arjungandhi/flights/pkg/db"
	"github.com/arjungandhi/flights/pkg/demo"
	"github.com/arjungandhi/flights/pkg/scan"
	"github.com/arjungandhi/flights/pkg/web"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	every := flag.Duration("every", 4*time.Hour, "scan interval")
	notify := flag.String("notify", "", `command that receives alert JSON lines on stdin, e.g. "messages send"; empty = stdout`)
	dir := flag.String("dir", db.DefaultDir(), "data directory ($FLIGHTS_DIR)")
	seed := flag.Bool("demo", false, "seed sample travelers/trips/offers into an empty database")
	noLoop := flag.Bool("no-scan", false, "serve the UI without the background scan loop")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: flights [flags]\n\nServes the web UI and runs the scanner.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	d, err := db.Open(*dir)
	if err != nil {
		log.Fatal(err)
	}
	defer d.Close()
	if *seed {
		if err := demo.Seed(d); err != nil {
			log.Fatalf("demo seed: %v", err)
		}
	}

	runner := &scan.Runner{DB: d}
	if *notify != "" {
		runner.Notify = scan.ExecNotifier{Command: *notify}
	} else {
		runner.Notify = scan.WriterNotifier{W: os.Stdout}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if !*noLoop {
		go runner.Loop(ctx, *every)
	}

	srv := &http.Server{Addr: *addr, Handler: (&web.Server{DB: d, Runner: runner, Every: *every}).Handler()}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Printf("flights: data %s, listening on %s", *dir, *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
