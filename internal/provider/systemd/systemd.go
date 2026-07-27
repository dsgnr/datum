// SPDX-License-Identifier: Apache-2.0

// Package systemd implements Service where systemd is the init system.
//
// Everything is read from one `systemctl show` call, because three separate
// queries could disagree with each other about a unit that changed between them.
package systemd

import (
	"context"
	"fmt"
	"strings"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/run"
	"github.com/dsgnr/datum/internal/state"
)

// Provider satisfies Service on a host running systemd.
type Provider struct {
	runner run.Runner
}

func New() *Provider {
	return &Provider{runner: run.Exec{}}
}

// NewWith returns a provider driving a supplied runner, which is how the tests run
// without systemd.
func NewWith(runner run.Runner) *Provider {
	return &Provider{runner: runner}
}

func (p *Provider) Name() string    { return "systemd" }
func (p *Provider) Types() []string { return []string{"Service"} }

// Detect reports whether systemd is the init system here.
//
// systemctl being installed is not enough, because it is present in plenty of
// container images where nothing booted it and every call fails on the bus. The
// directory is what systemd itself creates when it comes up as PID 1.
func Detect() bool {
	return run.Available("systemctl") && booted()
}

// unit is what one systemctl show call reports.
type unit struct {
	load       string
	active     string
	fileState  string
	properties map[string]string
}

func (u unit) exists() bool { return u.load != "not-found" && u.load != "" }

// running reports the unit as active, not as having a process, because a unit can be
// active while its main process restarts, and a process can exist while the unit has
// failed.
func (u unit) running() bool { return u.active == "active" }

// enabled reports whether the unit starts at boot, and whether that is a question
// with an answer. A static or masked unit has no install configuration, so enabling
// it is a no-op and comparing against it would report drift that can never clear.
func (u unit) enabled() (bool, bool) {
	switch u.fileState {
	case "enabled", "enabled-runtime":
		return true, true
	case "disabled":
		return false, true
	default:
		return false, false
	}
}

func (p *Provider) show(ctx context.Context, name string) (unit, error) {
	result, err := p.runner.Run(ctx, "systemctl", "show",
		"--property=LoadState", "--property=ActiveState", "--property=UnitFileState",
		"--", name)
	if err != nil {
		return unit{}, err
	}
	if !result.OK() {
		return unit{}, result.Err()
	}

	out := unit{properties: map[string]string{}}
	for _, line := range strings.Split(result.Stdout, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		out.properties[key] = value
	}
	out.load = out.properties["LoadState"]
	out.active = out.properties["ActiveState"]
	out.fileState = out.properties["UnitFileState"]
	return out, nil
}

// Observe reports whether the unit is known, running and enabled.
func (p *Provider) Observe(ctx context.Context, req provider.Request) (provider.Observation, error) {
	if err := validUnit(req.Target); err != nil {
		return provider.Observation{}, err
	}

	found, err := p.show(ctx, req.Target)
	if err != nil {
		return provider.Observation{}, err
	}
	if !found.exists() {
		return provider.Observation{Exists: false}, nil
	}

	observation := provider.Observation{
		Exists: true,
		Fields: map[string]document.Value{
			"state": document.Scalar(serviceState(found.running())),
			// Not declared by any document, so never compared. Reported because it
			// is what explains the two fields above.
			"loadState":     document.Scalar(found.load),
			"activeState":   document.Scalar(found.active),
			"unitFileState": document.Scalar(found.fileState),
		},
	}
	if enabled, known := found.enabled(); known {
		observation.Fields["enabled"] = document.Scalar(boolText(enabled))
	} else {
		observation.Unobservable = append(observation.Unobservable, "enabled")
	}
	if found.load == "masked" {
		observation.Found = "a masked unit"
	}
	return observation, nil
}

// Apply brings the unit to the declared state.
//
// The current state is read first, because starting a stopped unit and restarting a
// running one are different commands, and because a unit that is not there needs an
// error rather than an attempt.
func (p *Provider) Apply(ctx context.Context, req provider.Request, action state.Action) error {
	if err := validUnit(req.Target); err != nil {
		return err
	}

	switch action {
	case state.None, state.Skip:
		return nil
	case state.Remove:
		// Service has no absent state, so nothing should ever plan this.
		return fmt.Errorf("systemd: %s cannot be removed, because a Service does not own its unit", req.Ref)
	case state.Create, state.Update:
	default:
		return fmt.Errorf("systemd: unexpected action %s", action)
	}

	found, err := p.show(ctx, req.Target)
	if err != nil {
		return err
	}
	if !found.exists() {
		// Datum does not write units. The usual cause is a missing requires on
		// whatever installs it, which works on a host where the package is already
		// there and fails on a fresh one.
		return fmt.Errorf("systemd: unit %s is not known to systemd, so nothing installed it. "+
			"A Service needs a requires on the resource that provides its unit", req.Target)
	}

	if err := p.setEnabled(ctx, req, found); err != nil {
		return err
	}
	return p.setRunning(ctx, req, found)
}

// setEnabled makes the unit match the declared boot behaviour. A unit with no
// install configuration is left alone, because systemctl enable is a no-op on one
// and reporting success would be a lie either way.
func (p *Provider) setEnabled(ctx context.Context, req provider.Request, found unit) error {
	want, ok := boolField(req, "enabled")
	if !ok {
		return nil
	}
	current, known := found.enabled()
	if !known || current == want {
		return nil
	}

	verb := "disable"
	if want {
		verb = "enable"
	}
	return p.systemctl(ctx, verb, req.Target)
}

// setRunning makes the unit match the declared run state, and carries out a trigger
// where one fired.
func (p *Provider) setRunning(ctx context.Context, req provider.Request, found unit) error {
	if req.FieldOr("state", "running") == "stopped" {
		if !found.running() {
			return nil
		}
		return p.systemctl(ctx, "stop", req.Target)
	}

	// Starting a stopped unit satisfies a trigger as well, and reloading something
	// that is not running would fail for the wrong reason.
	if !found.running() {
		return p.systemctl(ctx, "start", req.Target)
	}

	switch req.Trigger {
	case provider.Reload:
		// A unit with no ExecReload fails here instead of being restarted, because
		// substituting a restart for a reload turns a stated requirement into an outage.
		return p.systemctl(ctx, "reload", req.Target)
	case provider.Restart:
		return p.systemctl(ctx, "restart", req.Target)
	default:
		// Already running, already enabled, nothing triggered.
		return nil
	}
}

func (p *Provider) systemctl(ctx context.Context, verb, unitName string) error {
	result, err := p.runner.Run(ctx, "systemctl", verb, "--", unitName)
	if err != nil {
		return err
	}
	return result.Err()
}

func serviceState(running bool) string {
	if running {
		return "running"
	}
	return "stopped"
}

func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// boolField reads a declared boolean. Field validation has already rejected
// anything that is not one of the two words.
func boolField(req provider.Request, name string) (bool, bool) {
	value, ok := req.Field(name)
	if !ok {
		return false, false
	}
	return value == "true", true
}

// validUnit rejects anything that is not a unit name.
//
// Arguments reach systemctl as a vector, so this is not what stops a shell
// metacharacter. It stops a name systemctl would read as an option, and one
// containing a path separator, which is a unit path rather than a unit.
func validUnit(name string) error {
	if name == "" {
		return fmt.Errorf("systemd: empty unit name")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("systemd: unit name %q would be read as an option", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == '@', r == ':', r == '\\':
		default:
			return fmt.Errorf("systemd: %q is not a unit name", name)
		}
	}
	return nil
}
