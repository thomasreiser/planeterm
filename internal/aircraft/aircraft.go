package aircraft

import (
	"sync"
	"time"
)

// Aircraft holds the most recently known state of a single transponder
// track; fields are merged across SBS messages, since a single MSG line
// usually carries only a subset of the fields
type Aircraft struct {
	ICAO      string
	Callsign  string
	Lat       float64
	Lon       float64
	Altitude  int     // feet
	Speed     float64 // knots ground speed
	Track     float64 // degrees, 0 = north, clockwise
	HasPos    bool
	FirstSeen time.Time
	LastSeen  time.Time
}

// Store is a thread-safe registry of aircraft keyed by ICAO hex
type Store struct {
	mu       sync.RWMutex
	aircraft map[string]*Aircraft
	ttl      time.Duration
}

func NewStore(ttl time.Duration) *Store {
	return &Store{
		aircraft: make(map[string]*Aircraft),
		ttl:      ttl,
	}
}

// Update merges new fields into the record for icao. The callback receives a
// pointer to the (possibly new) record; it should set the fields it knows
// about and leave the rest untouched
func (s *Store) Update(icao string, fn func(*Aircraft)) {
	if icao == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ac, ok := s.aircraft[icao]
	now := time.Now()
	if !ok {
		ac = &Aircraft{ICAO: icao, FirstSeen: now}
		s.aircraft[icao] = ac
	}
	fn(ac)
	ac.LastSeen = now
}

// Snapshot returns a copy of all aircraft seen within the TTL
// Caller owns the returned slice and may mutate it freely
func (s *Store) Snapshot() []Aircraft {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cutoff := time.Now().Add(-s.ttl)
	out := make([]Aircraft, 0, len(s.aircraft))
	for _, ac := range s.aircraft {
		if ac.LastSeen.Before(cutoff) {
			continue
		}
		out = append(out, *ac)
	}
	return out
}

// Prune drops records older than the TTL
func (s *Store) Prune() {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-s.ttl)
	for k, ac := range s.aircraft {
		if ac.LastSeen.Before(cutoff) {
			delete(s.aircraft, k)
		}
	}
}
