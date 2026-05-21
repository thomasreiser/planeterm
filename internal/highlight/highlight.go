// Package highlight loads callsign-based render rules from YAML and matches
// aircraft against them
package highlight

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Rule pairs a list of callsign prefixes with an RGB color
type Rule struct {
	Name     string   `yaml:"name"`
	Color    [3]int   `yaml:"color"`
	Prefixes []string `yaml:"prefixes"`
}

// Config is the on-disk format of highlight.yaml
type Config struct {
	Rules []Rule `yaml:"rules"`
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

// Match returns the first rule whose prefixes match callsign, or nil if
// none do..
func (c *Config) Match(callsign string) *Rule {
	if c == nil {
		return nil
	}
	cs := strings.ToUpper(strings.TrimSpace(callsign))
	if cs == "" {
		return nil
	}
	for i := range c.Rules {
		for _, p := range c.Rules[i].Prefixes {
			if p != "" && strings.HasPrefix(cs, p) {
				return &c.Rules[i]
			}
		}
	}
	return nil
}
