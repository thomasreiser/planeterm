// Minimal dump1090 SBS-1 (BaseStation) client that connects to the TCP feed and updates the shared aircraft store

// Field layout (0-indexed, only the ones we care about):
//
//	[0]  "MSG"
//	[1]  Transmission type (1-8)
//	[4]  HexIdent (ICAO 24-bit address)
//	[10] Callsign
//	[11] Altitude (feet)
//	[12] Ground speed (knots)
//	[13] Track (degrees)
//	[14] Latitude
//	[15] Longitude
package sbs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"planeterm/internal/aircraft"
)

type Client struct {
	addr      string
	store     *aircraft.Store
	connected atomic.Bool
	msgCount  atomic.Uint64
}

func New(addr string, store *aircraft.Store) *Client {
	return &Client{addr: addr, store: store}
}

func (c *Client) Connected() bool      { return c.connected.Load() }
func (c *Client) MessageCount() uint64 { return c.msgCount.Load() }
func (c *Client) Addr() string         { return c.addr }

// Run reconnects to the SBS server forever (until ctx is canceled),
// pushing messages into the store as they arrive
func (c *Client) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := c.readOnce(ctx)
		c.connected.Store(false)
		if errors.Is(err, context.Canceled) {
			return err
		}
		if err != nil {
			log.Printf("sbs: %v (retrying in %s)", err, backoff)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) readOnce(ctx context.Context) error {
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", c.addr, err)
	}
	defer conn.Close()
	c.connected.Store(true)

	// Cancel-aware: close the socket when ctx is done so the Scanner unblocks
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		c.parseLine(scanner.Text())
		c.msgCount.Add(1)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return errors.New("connection closed")
}

func (c *Client) parseLine(line string) {
	fields := strings.Split(line, ",")
	if len(fields) < 16 || fields[0] != "MSG" {
		return
	}
	icao := strings.TrimSpace(fields[4])
	if icao == "" {
		return
	}
	c.store.Update(icao, func(a *aircraft.Aircraft) {
		if v := strings.TrimSpace(fields[10]); v != "" {
			a.Callsign = v
		}
		if v := strings.TrimSpace(fields[11]); v != "" {
			if alt, err := strconv.Atoi(v); err == nil {
				a.Altitude = alt
			}
		}
		if v := strings.TrimSpace(fields[12]); v != "" {
			if spd, err := strconv.ParseFloat(v, 64); err == nil {
				a.Speed = spd
			}
		}
		if v := strings.TrimSpace(fields[13]); v != "" {
			if trk, err := strconv.ParseFloat(v, 64); err == nil {
				a.Track = trk
			}
		}
		latStr := strings.TrimSpace(fields[14])
		lonStr := strings.TrimSpace(fields[15])
		if latStr != "" && lonStr != "" {
			lat, errLat := strconv.ParseFloat(latStr, 64)
			lon, errLon := strconv.ParseFloat(lonStr, 64)
			if errLat == nil && errLon == nil {
				a.Lat = lat
				a.Lon = lon
				a.HasPos = true
			}
		}
	})
}
