package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"

	"planeterm/internal/aircraft"
	"planeterm/internal/highlight"
	"planeterm/internal/mil"
	"planeterm/internal/radar"
	"planeterm/internal/sbs"
)

func main() {
	host := flag.String("host", env("DUMP1090_HOST", "localhost"), "dump1090 host (SBS feed)")
	port := flag.String("port", env("DUMP1090_PORT", "30003"), "dump1090 SBS TCP port")
	lat := flag.Float64("lat", envFloat("RADAR_LAT", 49.699070), "radar center latitude (degrees)")
	lon := flag.Float64("lon", envFloat("RADAR_LON", 11.953948), "radar center longitude (degrees)")
	rangeNm := flag.Float64("range", envFloat("RADAR_RANGE_NM", 100), "radar range, nautical miles")
	ttl := flag.Duration("ttl", 60*time.Second, "drop aircraft after this long without an update")
	highlightPath := flag.String("highlight", env("HIGHLIGHT_FILE", "highlight.yaml"), "path to highlight rules YAML (missing file is OK)")
	milPath := flag.String("mil-file", env("MIL_FILE", "mil.yaml"), "path to military ICAO hex ranges YAML (missing file is OK)")
	flag.Parse()

	highlights, err := highlight.Load(*highlightPath)
	if err != nil {
		log.Printf("highlight: %v (continuing without rules)", err)
	}
	milRanges, err := mil.Load(*milPath)
	if err != nil {
		log.Printf("mil: %v (continuing without ranges)", err)
	}
	highlights.SetMil(milRanges)

	store := aircraft.NewStore(*ttl)
	addr := net.JoinHostPort(*host, *port)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	c := sbs.New(addr, store)
	var source radar.Source = c
	go func() {
		if err := c.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("sbs feed exited: %v", err)
		}
	}()

	// Periodic pruning so stale tracks fall off the scope
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				store.Prune()
			}
		}
	}()

	screen, err := tcell.NewScreen()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tcell:", err)
		os.Exit(1)
	}
	if err := screen.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "tcell init:", err)
		os.Exit(1)
	}
	defer screen.Fini()
	screen.SetStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))
	screen.Clear()

	r := radar.New(screen, store, source, *lat, *lon, *rangeNm, highlights)
	if err := r.Run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "radar:", err)
		os.Exit(1)
	}
}

// Try to read environment variable, return default if not set
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Try to parse environment variable as float, return default if not set or invalid
func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
