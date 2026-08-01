// SPDX-License-Identifier: Apache-2.0

// Package state holds the vocabulary a pass reports in. The names are fixed by the
// documentation, so they live in one place with nothing else in it.
package state

// Action is what a plan intends to do to one resource.
type Action int

const (
	// None means the resource already matches.
	None Action = iota
	Create
	Update
	Remove
	// Skip means nothing here can reconcile it, usually an unsupported type. Not a
	// failure.
	Skip
)

func (a Action) String() string {
	switch a {
	case Create:
		return "create"
	case Update:
		return "update"
	case Remove:
		return "remove"
	case Skip:
		return "skip"
	default:
		return "none"
	}
}

// Changes reports whether an action alters the host.
func (a Action) Changes() bool {
	return a == Create || a == Update || a == Remove
}

// Resource is where one resource stands within a pass.
type Resource int

const (
	Pending Resource = iota
	Applying
	Converged
	Drifted
	Failed
	// Blocked means a prerequisite failed, so this was never attempted.
	Blocked
	Skipped
)

func (r Resource) String() string {
	switch r {
	case Applying:
		return "applying"
	case Converged:
		return "converged"
	case Drifted:
		return "drifted"
	case Failed:
		return "failed"
	case Blocked:
		return "blocked"
	case Skipped:
		return "skipped"
	default:
		return "pending"
	}
}

// Host is the condition of a whole host across passes.
type Host int

const (
	// Unknown has not reported, which is not the same as reporting a problem.
	Unknown Host = iota
	HostConverged
	HostDrifted
	HostFailed
	// Degraded means part of the manifest cannot be reconciled here.
	Degraded
	AwaitingReboot
)

func (h Host) String() string {
	switch h {
	case HostConverged:
		return "converged"
	case HostDrifted:
		return "drifted"
	case HostFailed:
		return "failed"
	case Degraded:
		return "degraded"
	case AwaitingReboot:
		return "awaiting-reboot"
	default:
		return "unknown"
	}
}

// Outcome is how a pass ended.
type Outcome int

const (
	// OutcomeConverged means the plan was empty.
	OutcomeConverged Outcome = iota
	// OutcomeChanged means actions applied and every affected resource verified.
	OutcomeChanged
	OutcomeFailed
	// OutcomeDrifted only happens in observe mode, where a plan with actions in it is
	// reported rather than applied. Calling that converged would make the word useless,
	// and calling it failed would blame the host for doing as it was told.
	OutcomeDrifted
)

func (o Outcome) String() string {
	switch o {
	case OutcomeChanged:
		return "changed"
	case OutcomeFailed:
		return "failed"
	case OutcomeDrifted:
		return "drifted"
	default:
		return "converged"
	}
}

// HostFrom works out a host's condition from its resources. A failure outweighs a
// coverage gap, which outweighs drift.
func HostFrom(resources []Resource) Host {
	if len(resources) == 0 {
		return HostConverged
	}
	var failed, skipped, drifted bool
	for _, r := range resources {
		switch r {
		case Failed, Blocked:
			failed = true
		case Skipped:
			skipped = true
		case Drifted:
			drifted = true
		}
	}
	switch {
	case failed:
		return HostFailed
	case skipped:
		return Degraded
	case drifted:
		return HostDrifted
	default:
		return HostConverged
	}
}
