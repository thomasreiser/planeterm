// Package radar renders a PPI-style scope in the terminal: range rings, a
// crosshair, and every known aircraft drawn at once
package radar

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"

	"planeterm/internal/aircraft"
)

// Source is the minimum surface the radar needs to render the data-source
// section of the status bar
type Source interface {
	Connected() bool
	MessageCount() uint64
	Addr() string
}

type Radar struct {
	screen tcell.Screen
	store  *aircraft.Store
	source Source

	centerLat float64
	centerLon float64
	rangeNm   float64
}

func New(screen tcell.Screen, store *aircraft.Store, source Source, lat, lon, rangeNm float64) *Radar {
	return &Radar{
		screen:    screen,
		store:     store,
		source:    source,
		centerLat: lat,
		centerLon: lon,
		rangeNm:   rangeNm,
	}
}

const earthRadiusNm = 3440.065

func (r *Radar) Run(ctx context.Context) error {
	quit := make(chan struct{})
	go func() {
		defer close(quit)
		for {
			ev := r.screen.PollEvent()
			if ev == nil {
				return
			}
			switch ev := ev.(type) {
			case *tcell.EventKey:
				switch ev.Key() {
				case tcell.KeyEscape, tcell.KeyCtrlC:
					return
				}
				switch ev.Rune() {
				case 'q', 'Q':
					return
				case '+', '=':
					r.rangeNm = math.Max(5, r.rangeNm/1.25)
				case '-', '_':
					r.rangeNm = math.Min(1000, r.rangeNm*1.25)
				}
			case *tcell.EventResize:
				r.screen.Sync()
			}
		}
	}()

	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-quit:
			return nil
		case <-tick.C:
			r.draw()
			r.screen.Show()
		}
	}
}

func (r *Radar) draw() {
	s := r.screen
	s.Clear()

	w, h := s.Size()
	if w < 30 || h < 15 {
		drawText(s, 0, 0, "terminal too small", tcell.StyleDefault)
		return
	}

	plotH := h - 2 // reserve status bar
	cx := w / 2
	cy := plotH / 2

	// Cells are ~2× taller than wide, so a "visually round" circle uses
	// radius rows and 2·radius columns
	radius := plotH/2 - 2
	if hMax := (w/2 - 2) / 2; hMax < radius {
		radius = hMax
	}
	if radius < 4 {
		drawText(s, 0, 0, "terminal too small", tcell.StyleDefault)
		return
	}

	scopeBg := tcell.NewRGBColor(0, 12, 0)
	fillEllipse(s, cx, cy, radius, scopeBg)

	// Faint range rings at 25/50/75% and a brighter ring at the edge
	ringDim := tcell.StyleDefault.Foreground(tcell.NewRGBColor(0, 90, 0)).Background(scopeBg)
	for f := 0.25; f < 0.95; f += 0.25 {
		drawEllipse(s, cx, cy, int(math.Round(float64(radius)*f)), '·', ringDim)
	}
	ringEdge := tcell.StyleDefault.Foreground(tcell.NewRGBColor(0, 180, 0)).Background(scopeBg)
	drawEllipse(s, cx, cy, radius, '·', ringEdge)

	// Crosshair through center
	crossStyle := tcell.StyleDefault.Foreground(tcell.NewRGBColor(0, 70, 0)).Background(scopeBg)
	for dx := -2 * radius; dx <= 2*radius; dx++ {
		if dx == 0 {
			continue
		}
		setIfInside(s, cx+dx, cy, '─', crossStyle, cx, cy, radius)
	}
	for dy := -radius; dy <= radius; dy++ {
		if dy == 0 {
			continue
		}
		setIfInside(s, cx, cy+dy, '│', crossStyle, cx, cy, radius)
	}
	s.SetContent(cx, cy, '┼', nil, crossStyle)

	// Compass cardinals just outside the rim
	labelStyle := tcell.StyleDefault.Foreground(tcell.NewRGBColor(140, 220, 140)).Bold(true)
	drawText(s, cx, cy-radius-1, "N", labelStyle)
	drawText(s, cx, cy+radius+1, "S", labelStyle)
	drawText(s, cx-2*radius-2, cy, "W", labelStyle)
	drawText(s, cx+2*radius+1, cy, "E", labelStyle)

	// Aircraft tracks — every known contact, drawn at full brightness
	planeStyle := tcell.StyleDefault.Foreground(tcell.NewRGBColor(150, 255, 170)).Background(scopeBg).Bold(true)
	labelStyleA := tcell.StyleDefault.Foreground(tcell.NewRGBColor(180, 230, 180)).Background(scopeBg)
	inRange := 0
	snaps := r.store.Snapshot()
	for _, a := range snaps {
		if !a.HasPos {
			continue
		}
		dist, bearing := greatCircleBearing(r.centerLat, r.centerLon, a.Lat, a.Lon)
		if dist > r.rangeNm {
			continue
		}
		inRange++

		fracX := (dist / r.rangeNm) * math.Sin(bearing)
		fracY := -(dist / r.rangeNm) * math.Cos(bearing)
		ax := cx + int(math.Round(2*float64(radius)*fracX))
		ay := cy + int(math.Round(float64(radius)*fracY))

		s.SetContent(ax, ay, planeGlyph(a.Track), nil, planeStyle)

		label := strings.TrimSpace(a.Callsign)
		if label == "" {
			label = a.ICAO
		}
		if a.Altitude > 0 {
			label = fmt.Sprintf("%s FL%03d", label, a.Altitude/100)
		}
		// dist is in nautical miles; 1 NM = 1852 m exactly
		label = fmt.Sprintf("%s %s", label, formatDistance(int(math.Round(dist*1852))))
		drawText(s, ax+1, ay-1, label, labelStyleA)
	}

	r.drawStatus(w, h, len(snaps), inRange)
}

func (r *Radar) drawStatus(w, h, total, inRange int) {
	s := r.screen
	bg := tcell.NewRGBColor(0, 30, 0)
	st := tcell.StyleDefault.Foreground(tcell.NewRGBColor(200, 240, 200)).Background(bg)
	dim := tcell.StyleDefault.Foreground(tcell.NewRGBColor(120, 180, 120)).Background(bg)

	for x := 0; x < w; x++ {
		s.SetContent(x, h-2, ' ', nil, st)
		s.SetContent(x, h-1, ' ', nil, st)
	}

	src := r.source.Addr()
	connected := "OFFLINE"
	connStyle := tcell.StyleDefault.Foreground(tcell.NewRGBColor(255, 90, 90)).Background(bg).Bold(true)
	if r.source.Connected() {
		connected = "ONLINE"
		connStyle = tcell.StyleDefault.Foreground(tcell.NewRGBColor(120, 255, 140)).Background(bg).Bold(true)
	}

	left := fmt.Sprintf(" SRC %s  ", src)
	drawText(s, 0, h-2, left, dim)
	drawText(s, len(left), h-2, connected, connStyle)
	more := fmt.Sprintf("  msgs=%d", r.source.MessageCount())
	drawText(s, len(left)+len(connected), h-2, more, dim)

	right := fmt.Sprintf("RNG %.0f NM ", r.rangeNm)
	drawText(s, w-len(right), h-2, right, st)

	bottom := fmt.Sprintf(" TRK %d   IN RANGE %d   [+/-] zoom   [q] quit", total, inRange)
	drawText(s, 0, h-1, bottom, dim)
}

// formatDistance renders meters as "4,321 m" for short hops and as
// "12.3 km" once the distance crosses 5 km
func formatDistance(m int) string {
	if m < 0 {
		m = 0
	}
	if m >= 5000 {
		return fmt.Sprintf("%.1f km", float64(m)/1000)
	}
	s := fmt.Sprintf("%d", m)
	n := len(s)
	if n <= 3 {
		return s + " m"
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	b.WriteString(" m")
	return b.String()
}

func planeGlyph(track float64) rune {
	t := math.Mod(track, 360)
	if t < 0 {
		t += 360
	}
	sector := int(math.Mod((t+22.5)/45, 8))
	return [...]rune{'↑', '↗', '→', '↘', '↓', '↙', '←', '↖'}[sector]
}

func setIfInside(s tcell.Screen, x, y int, ch rune, st tcell.Style, cx, cy, r int) {
	dx := float64(x-cx) / 2
	dy := float64(y - cy)
	if dx*dx+dy*dy <= float64(r*r) {
		s.SetContent(x, y, ch, nil, st)
	}
}

func fillEllipse(s tcell.Screen, cx, cy, r int, bg tcell.Color) {
	st := tcell.StyleDefault.Background(bg)
	for dy := -r; dy <= r; dy++ {
		d := float64(r*r - dy*dy)
		if d < 0 {
			continue
		}
		dx := int(math.Round(2 * math.Sqrt(d)))
		for x := -dx; x <= dx; x++ {
			s.SetContent(cx+x, cy+dy, ' ', nil, st)
		}
	}
}

func drawEllipse(s tcell.Screen, cx, cy, r int, ch rune, st tcell.Style) {
	if r < 1 {
		return
	}
	n := int(2 * math.Pi * float64(r) * 2)
	if n < 32 {
		n = 32
	}
	for i := 0; i < n; i++ {
		θ := 2 * math.Pi * float64(i) / float64(n)
		x := cx + int(math.Round(2*float64(r)*math.Cos(θ)))
		y := cy + int(math.Round(float64(r)*math.Sin(θ)))
		s.SetContent(x, y, ch, nil, st)
	}
}

func drawLine(s tcell.Screen, x0, y0, x1, y1 int, ch rune, st tcell.Style) {
	dx := absInt(x1 - x0)
	dy := -absInt(y1 - y0)
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	x, y := x0, y0
	for {
		s.SetContent(x, y, ch, nil, st)
		if x == x1 && y == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x += sx
		}
		if e2 <= dx {
			err += dx
			y += sy
		}
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func drawText(s tcell.Screen, x, y int, text string, st tcell.Style) {
	w, h := s.Size()
	if y < 0 || y >= h {
		return
	}
	for _, ch := range text {
		if x >= w {
			return
		}
		if x >= 0 {
			s.SetContent(x, y, ch, nil, st)
		}
		x++
	}
}

func greatCircleBearing(lat1, lon1, lat2, lon2 float64) (distNm, bearingRad float64) {
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	dφ := (lat2 - lat1) * math.Pi / 180
	dλ := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dφ/2)*math.Sin(dφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(dλ/2)*math.Sin(dλ/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	distNm = earthRadiusNm * c
	y := math.Sin(dλ) * math.Cos(φ2)
	x := math.Cos(φ1)*math.Sin(φ2) - math.Sin(φ1)*math.Cos(φ2)*math.Cos(dλ)
	b := math.Atan2(y, x) // 0 = north, increases clockwise
	if b < 0 {
		b += 2 * math.Pi
	}
	bearingRad = b
	return
}
