// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dsgnr/datum/internal/config"
)

// minimal is the smallest file the reference says is usable, being the two keys with no
// default.
const minimal = `
host: web-001
source:
  url: https://git.example.com/fleet.git
`

func parse(t *testing.T, text string) (config.Config, []string) {
	t.Helper()
	cfg, warnings, err := config.Parse([]byte(text), "agent.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return cfg, warnings
}

func TestDefaultsMatchTheReference(t *testing.T) {
	cfg, warnings := parse(t, minimal)
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"source.branch", cfg.Source.Branch, "main"},
		{"source.fetchTimeout", time.Duration(cfg.Source.FetchTimeout), 5 * time.Minute},
		{"source.maxRepositorySize", cfg.Source.MaxRepositorySize.String(), "1GiB"},
		{"source.maxSourceSize", cfg.Source.MaxSourceSize.String(), "16MiB"},
		{"trust.signers", cfg.Trust.Signers, "/etc/datum/allowed-signers"},
		{"trust.require", cfg.Trust.Require, config.RequireSignedCommit},
		{"trust.requireDescendant", cfg.Trust.Descendant(), true},
		{"trust.strictPaths", cfg.Trust.StrictPaths, false},
		{"reconciliation.mode", cfg.Reconciliation.Mode, "enforce"},
		{"reconciliation.interval", time.Duration(cfg.Reconciliation.Interval), 30 * time.Minute},
		{"reconciliation.splay", cfg.Reconciliation.SplayOr(), 30 * time.Minute},
		{"reconciliation.timeout", time.Duration(cfg.Reconciliation.Timeout), 15 * time.Minute},
		{"reconciliation.actionTimeout", time.Duration(cfg.Reconciliation.ActionTimeout), 5 * time.Minute},
		{"metrics.listen", cfg.Metrics.Listen, "127.0.0.1:10056"},
		{"state", cfg.State, "/var/lib/datum"},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s = %v, want %v", check.name, check.got, check.want)
		}
	}
}

// The whole file from the reference page has to load, or the reference and the code
// have diverged.
func TestTheReferenceExampleLoads(t *testing.T) {
	const whole = `
host: web-001

source:
  url: https://git.example.com/fleet.git
  branch: main
  credential: /etc/datum/credentials/git
  fetchTimeout: 5m
  maxRepositorySize: 1GiB
  maxSourceSize: 16MiB

trust:
  signers: /etc/datum/allowed-signers
  require: signed-tag
  tagPattern: "release-*"
  requireDescendant: true
  strictPaths: false

reconciliation:
  mode: enforce
  interval: 30m
  splay: 30m
  timeout: 15m
  actionTimeout: 5m

secrets:
  provider: file
  path: /etc/datum/secrets

metrics:
  listen: 127.0.0.1:10056
  textfile: /var/lib/node_exporter/textfile/datum.prom

state: /var/lib/datum
`
	cfg, warnings := parse(t, whole)
	if len(warnings) != 0 {
		t.Errorf("the reference example produced warnings: %v", warnings)
	}
	if cfg.Trust.Require != config.RequireSignedTag {
		t.Errorf("trust.require = %q", cfg.Trust.Require)
	}
	if cfg.Secrets.Path != "/etc/datum/secrets" {
		t.Errorf("secrets.path = %q", cfg.Secrets.Path)
	}
	if cfg.Metrics.Textfile == "" {
		t.Error("metrics.textfile was dropped")
	}
}

func TestHostIsRequired(t *testing.T) {
	_, _, err := config.Parse([]byte("source:\n  url: x\n"), "agent.yaml")
	if err == nil {
		t.Fatal("a file with no host was accepted")
	}
	if !strings.Contains(err.Error(), "host is required") {
		t.Errorf("error = %v", err)
	}
}

// Zero splay is a real setting, so it has to survive being indistinguishable from
// absent in a plain struct.
func TestZeroSplayIsNotTheSameAsAbsent(t *testing.T) {
	cfg, _ := parse(t, minimal+"\nreconciliation:\n  splay: 0s\n")
	if got := cfg.Reconciliation.SplayOr(); got != 0 {
		t.Errorf("splay = %s, want 0", got)
	}

	absent, _ := parse(t, minimal)
	if got := absent.Reconciliation.SplayOr(); got != 30*time.Minute {
		t.Errorf("absent splay = %s, want the interval", got)
	}
}

// requireDescendant defaults on, so false has to be distinguishable from absent for
// the same reason.
func TestRequireDescendantCanBeTurnedOff(t *testing.T) {
	cfg, _ := parse(t, minimal+"\ntrust:\n  requireDescendant: false\n")
	if cfg.Trust.Descendant() {
		t.Error("requireDescendant: false was ignored")
	}
}

func TestAPassCannotOutliveItsInterval(t *testing.T) {
	_, _, err := config.Parse([]byte(minimal+"\nreconciliation:\n  interval: 10m\n  timeout: 20m\n"), "agent.yaml")
	if err == nil {
		t.Fatal("a timeout longer than the interval was accepted")
	}
	if !strings.Contains(err.Error(), "longer than") {
		t.Errorf("error = %v", err)
	}
}

func TestSignedTagNeedsAPattern(t *testing.T) {
	_, _, err := config.Parse([]byte(minimal+"\ntrust:\n  require: signed-tag\n"), "agent.yaml")
	if err == nil {
		t.Fatal("signed-tag with no pattern was accepted")
	}
}

func TestBadValuesAreRefused(t *testing.T) {
	for name, text := range map[string]string{
		"unknown require": minimal + "\ntrust:\n  require: maybe\n",
		"unknown mode":    minimal + "\nreconciliation:\n  mode: perhaps\n",
		"zero interval":   minimal + "\nreconciliation:\n  interval: 0s\n",
		"bad duration":    minimal + "\nreconciliation:\n  interval: soon\n",
		"bad size":        minimal + "\nsource:\n  maxSourceSize: enormous\n",
		"unknown secrets": minimal + "\nsecrets:\n  provider: vault\n",
	} {
		if _, _, err := config.Parse([]byte(text), "agent.yaml"); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A typo should be visible without stopping the host reconciling, because this file
// is not reviewed in a pull request and refusing would strand a host on an upgrade.
func TestUnknownKeysWarnRatherThanRefuse(t *testing.T) {
	cfg, warnings := parse(t, minimal+"\nreconcilation:\n  interval: 10m\nmetrics:\n  lisen: none\n")
	if cfg.Host != "web-001" {
		t.Error("the file did not load")
	}
	joined := strings.Join(warnings, "; ")
	for _, want := range []string{`"reconcilation"`, `"metrics.lisen"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings %q do not mention %s", joined, want)
		}
	}
	// The misspelling must not have silently changed the real setting.
	if time.Duration(cfg.Reconciliation.Interval) != 30*time.Minute {
		t.Error("a misspelled key changed a real value")
	}
}

func TestListenNoneDisablesTheListener(t *testing.T) {
	cfg, _ := parse(t, minimal+"\nmetrics:\n  listen: none\n")
	if cfg.Metrics.Serving() {
		t.Error("listen: none still serves")
	}
	on, _ := parse(t, minimal)
	if !on.Metrics.Serving() {
		t.Error("the default does not serve")
	}
}

func TestDurationsAndSizes(t *testing.T) {
	for text, want := range map[string]time.Duration{
		"30m":    30 * time.Minute,
		"1h30m":  90 * time.Minute,
		"1d":     24 * time.Hour,
		"90s":    90 * time.Second,
		"15m30s": 15*time.Minute + 30*time.Second,
	} {
		got, err := config.ParseDuration(text)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %s, %v, want %s", text, got, err, want)
		}
	}

	for text, want := range map[string]config.Size{
		"1GiB":  1 << 30,
		"16MiB": 16 << 20,
		"1024":  1024,
		"4KiB":  4 << 10,
		"1MB":   1 << 20,
		"512B":  512,
	} {
		got, err := config.ParseSize(text)
		if err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v, want %d", text, got, err, want)
		}
	}

	if got := config.Size(1 << 30).String(); got != "1GiB" {
		t.Errorf("Size.String = %q", got)
	}
}

// The reference writes 30m, and a command that echoes 30m0s back at somebody reading
// the reference looks like it parsed something else.
func TestDurationsPrintTheWayTheReferenceWritesThem(t *testing.T) {
	for text, want := range map[string]string{
		"30m":    "30m",
		"15m":    "15m",
		"1h":     "1h",
		"5m":     "5m",
		"90s":    "1m30s",
		"15m30s": "15m30s",
		"2h30m":  "2h30m",
	} {
		parsed, err := config.ParseDuration(text)
		if err != nil {
			t.Fatalf("ParseDuration(%q): %v", text, err)
		}
		if got := config.Duration(parsed).String(); got != want {
			t.Errorf("Duration(%q).String() = %q, want %q", text, got, want)
		}
	}
}
