// SPDX-License-Identifier: Apache-2.0

// Package dnf implements Package on Fedora, RHEL and their relatives.
//
// Two differences from apt affect the implementation. rpm reports an absent package on
// stdout rather than stderr, and dnf has no single command that moves a package to an
// exact version in either direction.
package dnf

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/run"
	"github.com/dsgnr/datum/internal/state"
)

// Provider satisfies Package where rpm and dnf are the package manager.
type Provider struct {
	runner run.Runner
}

func New() *Provider {
	return &Provider{runner: run.Exec{}}
}

// NewWith returns a provider driving a supplied runner, which is how the tests run
// without rpm.
func NewWith(runner run.Runner) *Provider {
	return &Provider{runner: runner}
}

func (p *Provider) Name() string    { return "dnf" }
func (p *Provider) Types() []string { return []string{"Package"} }

// versionFormat is the form rpm reports and dnf accepts in a name-version argument.
// Epoch is left out because it is usually unset and dnf takes the argument without it.
const versionFormat = `--queryformat=%{VERSION}-%{RELEASE}\n`

// Observe reports whether the package is installed and at which version.
//
// rpm exits 1 for a package that is not installed and says so on stdout, which is
// where apt's assumption of stderr would have quietly turned every absent package
// into a failure.
func (p *Provider) Observe(ctx context.Context, req provider.Request) (provider.Observation, error) {
	if err := validName(req.Target); err != nil {
		return provider.Observation{}, err
	}

	result, err := p.runner.Run(ctx, "rpm", "--query", versionFormat, "--", req.Target)
	if err != nil {
		return provider.Observation{}, err
	}
	if !result.OK() {
		if notInstalled(result.Stdout + result.Stderr) {
			return provider.Observation{Exists: false}, nil
		}
		return provider.Observation{}, result.Err()
	}

	return provider.Observation{
		Exists: true,
		Fields: map[string]document.Value{
			"version": document.Scalar(strings.TrimSpace(result.Stdout)),
		},
	}, nil
}

// Apply installs, moves or removes the package.
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
		return fmt.Errorf("dnf: unexpected action %s", action)
	}
}

// install brings the package to the declared version.
//
// dnf install upgrades and will not go backwards, so a pinned version that is lower
// than the installed one needs dnf downgrade. Which of the two it is comes from rpm's
// own comparison, not from anything invented here, because rpm version ordering has
// rules a string compare gets wrong.
func (p *Provider) install(ctx context.Context, req provider.Request) error {
	version, pinned := req.Field("version")
	if !pinned || version == "" {
		return p.dnf(ctx, "install", req.Target)
	}
	if err := validVersion(version); err != nil {
		return err
	}

	verb := "install"
	current, installed, err := p.installedVersion(ctx, req.Target)
	if err != nil {
		return err
	}
	if installed {
		order, err := p.compare(ctx, version, current)
		if err != nil {
			return err
		}
		switch {
		case order == 0:
			// Already exactly right. Verification will confirm it.
			return nil
		case order < 0:
			verb = "downgrade"
		}
	}
	return p.dnf(ctx, verb, req.Target+"-"+version)
}

func (p *Provider) remove(ctx context.Context, req provider.Request) error {
	return p.dnf(ctx, "remove", req.Target)
}

// dnf runs one transaction.
//
// There is no `--` separator here, because dnf5 rejects it as an unknown argument
// where apt-get accepts it. validName refusing a leading dash is what stops a package
// name being read as an option instead, which is why that check is not decoration.
func (p *Provider) dnf(ctx context.Context, verb, target string) error {
	result, err := p.runner.Run(ctx, "dnf", verb,
		"--assumeyes",
		// A transaction that cannot be resolved has to fail instead of quietly doing part of
		// what was asked.
		"--setopt=strict=1",
		target)
	if err != nil {
		return err
	}
	return result.Err()
}

func (p *Provider) installedVersion(ctx context.Context, name string) (string, bool, error) {
	result, err := p.runner.Run(ctx, "rpm", "--query", versionFormat, "--", name)
	if err != nil {
		return "", false, err
	}
	if !result.OK() {
		if notInstalled(result.Stdout + result.Stderr) {
			return "", false, nil
		}
		return "", false, result.Err()
	}
	return strings.TrimSpace(result.Stdout), true, nil
}

// compare orders two rpm versions using rpm's own comparison, which is the only thing
// that gets cases like 1.0 against 1.0~rc1 right.
//
// The versions are interpolated into an expression rpm evaluates as Lua, so
// validVersion rejecting quotes and parentheses is doing a second job here beyond
// keeping dnf from reading an argument as an option.
func (p *Provider) compare(ctx context.Context, a, b string) (int, error) {
	if err := validVersion(a); err != nil {
		return 0, err
	}
	if err := validVersion(b); err != nil {
		return 0, err
	}

	expression := fmt.Sprintf(`%%{lua: print(rpm.vercmp(%q, %q))}`, a, b)
	result, err := p.runner.Run(ctx, "rpm", "--eval", expression)
	if err != nil {
		return 0, err
	}
	if !result.OK() {
		return 0, result.Err()
	}

	order, err := strconv.Atoi(strings.TrimSpace(result.Stdout))
	if err != nil {
		return 0, fmt.Errorf("dnf: rpm compared %s and %s as %q, which is not an ordering",
			a, b, strings.TrimSpace(result.Stdout))
	}
	return order, nil
}

// notInstalled matches what rpm says about a package it has no record of. rpm puts it
// on stdout, unlike dpkg-query.
func notInstalled(output string) bool {
	return strings.Contains(strings.ToLower(output), "is not installed")
}

// An rpm package name is alphanumeric plus these.
func validName(name string) error {
	if err := run.Word("package name", name, "+-._"); err != nil {
		return fmt.Errorf("dnf: %w", err)
	}
	return nil
}

// An rpm version is alphanumeric plus these. Tilde and caret order releases. The quotes
// and parentheses that mean something to rpm's Lua evaluator are left out.
func validVersion(version string) error {
	if err := run.Word("version", version, "+-._:~^"); err != nil {
		return fmt.Errorf("dnf: %w", err)
	}
	return nil
}
