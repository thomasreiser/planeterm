// Package highlight loads callsign- and hex-based render rules from YAML
// and matches aircraft against them
package highlight

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"planeterm/internal/mil"
)

// Rule pairs callsign prefixes (and optionally the loaded mil hex ranges)
// with an RGB color
type Rule struct {
	Name     string   `yaml:"name"`
	Color    [3]int   `yaml:"color"`
	Prefixes []string `yaml:"prefixes"`
	Mil      bool     `yaml:"mil"` // also match if ICAO is in the mil ranges
}

// Config is the on-disk format of highlight.yaml
type Config struct {
	Rules []Rule `yaml:"rules"`

	mil *mil.Config
}

// Load reads and parses a highlight YAML file
// Ignores missing files, but returns an error for malformed ones
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
	// Pre-uppercase prefixes once so Match doesn't allocate per call
	for i := range c.Rules {
		for j, p := range c.Rules[i].Prefixes {
			c.Rules[i].Prefixes[j] = strings.ToUpper(strings.TrimSpace(p))
		}
	}
	return &c, nil
}

// SetMil attaches the loaded mil ranges so rules with `mil: true` can fire
// on hex membership. Safe to call with nil.
func (c *Config) SetMil(m *mil.Config) {
	if c == nil {
		return
	}
	c.mil = m
}

// Match returns the first rule that matches the given callsign or icao,
// or nil if none do.
func (c *Config) Match(callsign, icao string) *Rule {
	if c == nil {
		return nil
	}
	cs := strings.ToUpper(strings.TrimSpace(callsign))
	for i := range c.Rules {
		r := &c.Rules[i]
		if r.Mil && c.mil.Contains(icao) {
			return r
		}
		if cs != "" {
			for _, p := range r.Prefixes {
				if p != "" && strings.HasPrefix(cs, p) {
					return r
				}
			}
		}
	}
	return nil
}
