// SPDX-License-Identifier: Apache-2.0

// Package apt implements Package on Debian and its derivatives.
//
// Reading goes through dpkg-query, which reports what is installed without
// touching the network. Changing goes through apt-get.
package apt

import (
	"context"
	"fmt"
	"strings"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/run"
	"github.com/dsgnr/datum/internal/state"
)

// Provider satisfies Package where dpkg and apt-get are present.
type Provider struct {
	runner run.Runner
}

// New returns a provider driving the real programs.
func New() *Provider {
	// apt-get prompts on some upgrades and conffile changes, and a prompt in a
	// pass is a hang. The frontend setting is the documented way to refuse.
	return &Provider{runner: run.Exec{}.With(map[string]string{
		"DEBIAN_FRONTEND": "noninteractive",
	})}
}

// NewWith returns a provider driving a supplied runner, which is how the tests run
// without dpkg.
func NewWith(runner run.Runner) *Provider {
	return &Provider{runner: runner}
}

func (p *Provider) Name() string    { return "apt" }
func (p *Provider) Types() []string { return []string{"Package"} }

// Detect reports whether this host is one apt manages.
func Detect() bool {
	return run.Available("dpkg-query") && run.Available("apt-get")
}

// Observe reports whether the package is installed and at which version.
//
// dpkg-query exits non-zero for a package it has never heard of, which is the common
// case for something not installed, not an error.
func (p *Provider) Observe(ctx context.Context, req provider.Request) (provider.Observation, error) {
	if err := validName(req.Target); err != nil {
		return provider.Observation{}, err
	}

	result, err := p.runner.Run(ctx, "dpkg-query",
		"--showformat=${db:Status-Status}\\n${Version}\\n", "--show", req.Target)
	if err != nil {
		return provider.Observation{}, err
	}
	if !result.OK() {
		// Distinguishing "not installed" from a broken dpkg matters, and the only signal is
		// the message. Anything else is reported, never guessed at.
		if unknownPackage(result.Stderr) {
			return provider.Observation{Exists: false}, nil
		}
		return provider.Observation{}, result.Err()
	}

	status, version := parseShow(result.Stdout)
	if status != "installed" {
		// Removed but not purged still has configuration on disk. For the purposes
		// of a Package resource it is not installed, and the status is recorded so
		// a plan can say why it is being installed again.
		found := ""
		if status != "" && status != "not-installed" {
			found = "a package in state " + status
		}
		return provider.Observation{Exists: false, Found: found}, nil
	}

	return provider.Observation{
		Exists: true,
		Fields: map[string]document.Value{
			"version": document.Scalar(version),
		},
	}, nil
}

// Apply installs, changes or removes the package.
func (p *Provider) Apply(ctx context.Context, req provider.Request, action state.Action) error {
	if err := validName(req.Target); err != nil {
		return err
	}

	switch action {
	case state.Create, state.Update:
		return p.install(ctx, req)
	case state.Remove:
		return p.remove(ctx, req)
	case state.None, state.Skip:
		return nil
	default:
		return fmt.Errorf("apt: unexpected action %s", action)
	}
}

// install handles both a missing package and one at the wrong version. apt-get
// takes them the same way, and `name=version` moves a package downwards as well as
// up, which is what pinning a version has to mean.
func (p *Provider) install(ctx context.Context, req provider.Request) error {
	target := req.Target
	if version, ok := req.Field("version"); ok && version != "" {
		if err := validVersion(version); err != nil {
			return err
		}
		target = req.Target + "=" + version
	}

	argv := []string{"apt-get", "install",
		"--yes",
		// A conffile prompt cannot be answered during a pass. Keeping the
		// installed version is the safe half of that choice, and a file Datum
		// manages is written by its own File resource regardless.
		"--option", "Dpkg::Options::=--force-confold",
		"--option", "Dpkg::Options::=--force-confdef",
		// Downgrading is refused without this, and a pinned version has to be
		// reachable from either direction.
		"--allow-downgrades",
		target,
	}
	result, err := p.runner.Run(ctx, argv...)
	if err != nil {
		return err
	}
	return result.Err()
}

// remove uninstalls without purging. Purging deletes configuration that Datum did
// not put there and cannot put back, so it is not the default for a resource that
// only says the package should be absent.
func (p *Provider) remove(ctx context.Context, req provider.Request) error {
	result, err := p.runner.Run(ctx, "apt-get", "remove", "--yes", req.Target)
	if err != nil {
		return err
	}
	return result.Err()
}

// parseShow reads the two lines dpkg-query was asked for. A package with no version
// yields an empty one rather than an error, because the status is the field that
// decides whether it exists.
func parseShow(out string) (status, version string) {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 0 {
		status = strings.TrimSpace(lines[0])
	}
	if len(lines) > 1 {
		version = strings.TrimSpace(lines[1])
	}
	return status, version
}

func unknownPackage(stderr string) bool {
	lowered := strings.ToLower(stderr)
	return strings.Contains(lowered, "no packages found matching") ||
		strings.Contains(lowered, "not found")
}

// A Debian package name is alphanumeric plus these, and an equals sign would turn one
// argument into a version specifier.
func validName(name string) error {
	if err := run.Word("package name", name, "+-._:"); err != nil {
		return fmt.Errorf("apt: %w", err)
	}
	return nil
}

func validVersion(version string) error {
	if err := run.Word("version", version, "+-.:~"); err != nil {
		return fmt.Errorf("apt: %w", err)
	}
	return nil
}
