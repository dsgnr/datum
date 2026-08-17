// SPDX-License-Identifier: Apache-2.0

// Package config reads the agent configuration file.
//
// The file is a trust anchor, so nothing in a repository can change any of it and Datum
// refuses to manage it. Every key has a default except the host name and the source
// URL, which is why loading is mostly defaults and hardly any parsing.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Path is where the agent looks unless told otherwise.
const Path = "/etc/datum/agent.yaml"

// Defaults, kept together because the reference page lists them as a table and they
// are easier to check against it in one place than spread through the struct.
const (
	DefaultBranch            = "main"
	DefaultFetchTimeout      = 5 * time.Minute
	DefaultMaxRepositorySize = 1 << 30 // 1GiB
	DefaultMaxSourceSize     = 16 << 20
	DefaultSigners           = "/etc/datum/allowed-signers"
	DefaultRequire           = RequireSignedCommit
	DefaultMode              = "enforce"
	DefaultInterval          = 30 * time.Minute
	DefaultTimeout           = 15 * time.Minute
	DefaultActionTimeout     = 5 * time.Minute
	DefaultListen            = "127.0.0.1:10056"
	DefaultState             = "/var/lib/datum"
)

// Require is how much a revision has to prove before it is applied.
type Require string

const (
	RequireSignedCommit Require = "signed-commit"
	RequireSignedTag    Require = "signed-tag"
	RequireNone         Require = "none"
)

// Config is the whole file, resolved. Every field holds the value that applies,
// never the empty value meaning "use the default", so nothing downstream repeats
// defaulting logic or gets it slightly wrong.
type Config struct {
	Host           string         `yaml:"host"`
	Source         Source         `yaml:"source"`
	Trust          Trust          `yaml:"trust"`
	Reconciliation Reconciliation `yaml:"reconciliation"`
	Secrets        Secrets        `yaml:"secrets"`
	Metrics        Metrics        `yaml:"metrics"`
	State          string         `yaml:"state"`
}

type Source struct {
	URL               string   `yaml:"url"`
	Branch            string   `yaml:"branch"`
	Credential        string   `yaml:"credential"`
	FetchTimeout      Duration `yaml:"fetchTimeout"`
	MaxRepositorySize Size     `yaml:"maxRepositorySize"`
	MaxSourceSize     Size     `yaml:"maxSourceSize"`
}

type Trust struct {
	Signers           string  `yaml:"signers"`
	Require           Require `yaml:"require"`
	TagPattern        string  `yaml:"tagPattern"`
	RequireDescendant *bool   `yaml:"requireDescendant"`
	StrictPaths       bool    `yaml:"strictPaths"`
}

// Descendant reports whether a revision has to descend from the accepted one. It
// defaults on, so the field is a pointer to tell "absent" from "false".
func (t Trust) Descendant() bool {
	return t.RequireDescendant == nil || *t.RequireDescendant
}

type Reconciliation struct {
	Mode          string    `yaml:"mode"`
	Interval      Duration  `yaml:"interval"`
	Splay         *Duration `yaml:"splay"`
	Timeout       Duration  `yaml:"timeout"`
	ActionTimeout Duration  `yaml:"actionTimeout"`
}

// SplayOr returns the spread, which defaults to the interval and not to zero. A zero
// splay is meaningful, so absence and zero have to stay distinguishable.
func (r Reconciliation) SplayOr() time.Duration {
	if r.Splay == nil {
		return time.Duration(r.Interval)
	}
	return time.Duration(*r.Splay)
}

type Secrets struct {
	Provider string `yaml:"provider"`
	Path     string `yaml:"path"`
}

type Metrics struct {
	Listen   string `yaml:"listen"`
	Textfile string `yaml:"textfile"`
}

// Serving reports whether the listener is on. The reference spells disabling it as the
// word none rather than an empty string, so that turning it off is a statement.
func (m Metrics) Serving() bool {
	return m.Listen != "" && m.Listen != "none"
}

// Default is the configuration a file sets out to change.
func Default() Config {
	descendant := true
	return Config{
		Source: Source{
			Branch:            DefaultBranch,
			FetchTimeout:      Duration(DefaultFetchTimeout),
			MaxRepositorySize: DefaultMaxRepositorySize,
			MaxSourceSize:     DefaultMaxSourceSize,
		},
		Trust: Trust{
			Signers:           DefaultSigners,
			Require:           DefaultRequire,
			RequireDescendant: &descendant,
		},
		Reconciliation: Reconciliation{
			Mode:          DefaultMode,
			Interval:      Duration(DefaultInterval),
			Timeout:       Duration(DefaultTimeout),
			ActionTimeout: Duration(DefaultActionTimeout),
		},
		Metrics: Metrics{Listen: DefaultListen},
		State:   DefaultState,
	}
}

// Load reads and validates a configuration file.
//
// Unknown keys come back as warnings, not errors. Refusing would make a typo loud, and
// it would also stop a host reconciling the moment somebody adds a key a newer agent
// understands. This file is not reviewed in a pull request.
func Load(path string) (Config, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, nil, err
	}
	return Parse(data, path)
}

// Parse resolves configuration from the bytes of a file.
func Parse(data []byte, path string) (Config, []string, error) {
	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, nil, fmt.Errorf("%s: %w", path, err)
	}

	warnings := unknownKeys(data)

	// An empty string in the file is the same as the key being absent, or a comment
	// left in place would silently clear a default.
	if cfg.Source.Branch == "" {
		cfg.Source.Branch = DefaultBranch
	}
	if cfg.Trust.Signers == "" {
		cfg.Trust.Signers = DefaultSigners
	}
	if cfg.Trust.Require == "" {
		cfg.Trust.Require = DefaultRequire
	}
	if cfg.Reconciliation.Mode == "" {
		cfg.Reconciliation.Mode = DefaultMode
	}
	if cfg.State == "" {
		cfg.State = DefaultState
	}

	if err := cfg.Validate(path); err != nil {
		return Config{}, warnings, err
	}
	return cfg, warnings, nil
}

// Validate refuses a file that cannot describe a working agent.
func (c Config) Validate(path string) error {
	if c.Host == "" {
		return fmt.Errorf("%s: host is required, because a machine has to claim a name a Host document matches", path)
	}
	switch c.Trust.Require {
	case RequireSignedCommit, RequireSignedTag, RequireNone:
	default:
		return fmt.Errorf("%s: trust.require is %q, want signed-commit, signed-tag or none", path, c.Trust.Require)
	}
	if c.Trust.Require == RequireSignedTag && c.Trust.TagPattern == "" {
		return fmt.Errorf("%s: trust.require is signed-tag, so trust.tagPattern has to say which tags are candidates", path)
	}
	switch c.Reconciliation.Mode {
	case "enforce", "observe":
	default:
		return fmt.Errorf("%s: reconciliation.mode is %q, want enforce or observe", path, c.Reconciliation.Mode)
	}
	if time.Duration(c.Reconciliation.Interval) <= 0 {
		return fmt.Errorf("%s: reconciliation.interval has to be positive", path)
	}
	if time.Duration(c.Reconciliation.Timeout) <= 0 {
		return fmt.Errorf("%s: reconciliation.timeout has to be positive", path)
	}
	// A pass that outlives its own interval turns the skipped tick from an exception into
	// the normal case, so this is refused, not warned about.
	if time.Duration(c.Reconciliation.Timeout) > time.Duration(c.Reconciliation.Interval) {
		return fmt.Errorf("%s: reconciliation.timeout (%s) is longer than reconciliation.interval (%s), so a pass would never finish before its successor is due",
			path, c.Reconciliation.Timeout, c.Reconciliation.Interval)
	}
	if c.Reconciliation.SplayOr() < 0 {
		return fmt.Errorf("%s: reconciliation.splay cannot be negative", path)
	}
	if c.Secrets.Provider != "" && c.Secrets.Provider != "file" {
		return fmt.Errorf("%s: secrets.provider is %q, and only file is implemented", path, c.Secrets.Provider)
	}
	return nil
}

// unknownKeys reports keys that no field claims, as a warning per key.
func unknownKeys(data []byte) []string {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}
	known := map[string][]string{
		"host":           nil,
		"source":         {"url", "branch", "credential", "fetchTimeout", "maxRepositorySize", "maxSourceSize"},
		"trust":          {"signers", "require", "tagPattern", "requireDescendant", "strictPaths"},
		"reconciliation": {"mode", "interval", "splay", "timeout", "actionTimeout"},
		"secrets":        {"provider", "path"},
		"metrics":        {"listen", "textfile"},
		"state":          nil,
	}

	var warnings []string
	for key, value := range raw {
		children, ok := known[key]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("unknown key %q", key))
			continue
		}
		block, ok := value.(map[string]any)
		if !ok || children == nil {
			continue
		}
		for child := range block {
			if !contains(children, child) {
				warnings = append(warnings, fmt.Sprintf("unknown key %q", key+"."+child))
			}
		}
	}
	sortStrings(warnings)
	return warnings
}

func contains(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

// sortStrings keeps warnings in a stable order, because map iteration is not and a
// command that prints them should not reorder between runs.
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
