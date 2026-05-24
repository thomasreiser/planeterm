// Package mil loads named ICAO 24-bit address ranges (typically military
// allocations) from YAML and offers fast membership lookup
package mil

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Range is one inclusive ICAO hex span
type Range struct {
	Name  string `yaml:"name"`
	Start string `yaml:"start"`
	End   string `yaml:"end"`

	start uint32
	end   uint32
}

// Config is the on-disk format of a mil ranges file
type Config struct {
	Ranges []Range `yaml:"ranges"`
}

// Load reads and parses a mil ranges YAML file
func Load(path string) (*Config, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for i := range c.Ranges {
		s, err := parseHex(c.Ranges[i].Start)
		if err != nil {
			return nil, fmt.Errorf("%s: start: %w", c.Ranges[i].Name, err)
		}
		e, err := parseHex(c.Ranges[i].End)
		if err != nil {
			return nil, fmt.Errorf("%s: end: %w", c.Ranges[i].Name, err)
		}
		if e < s {
			return nil, fmt.Errorf("%s: end %s < start %s", c.Ranges[i].Name, c.Ranges[i].End, c.Ranges[i].Start)
		}
		c.Ranges[i].start = s
		c.Ranges[i].end = e
	}
	return &c, nil
}

// Contains reports whether the given ICAO hex falls in any loaded range
func (c *Config) Contains(icao string) bool {
	if c == nil || len(c.Ranges) == 0 {
		return false
	}
	v, err := parseHex(icao)
	if err != nil {
		return false
	}
	for _, r := range c.Ranges {
		if v >= r.start && v <= r.end {
			return true
		}
	}
	return false
}

// Count returns the number of ranges loaded (for logging)
func (c *Config) Count() int {
	if c == nil {
		return 0
	}
	return len(c.Ranges)
}

func parseHex(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("not a hex value: %q", s)
	}
	return uint32(v), nil
}
