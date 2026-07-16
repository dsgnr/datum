// SPDX-License-Identifier: Apache-2.0

package state

import "testing"

func TestActionStrings(t *testing.T) {
	cases := map[Action]string{
		None: "none", Create: "create", Update: "update", Remove: "remove", Skip: "skip",
	}
	for action, want := range cases {
		if got := action.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", action, got, want)
		}
	}
}

func TestOnlySomeActionsChangeTheHost(t *testing.T) {
	for _, a := range []Action{Create, Update, Remove} {
		if !a.Changes() {
			t.Errorf("%s should change the host", a)
		}
	}
	for _, a := range []Action{None, Skip} {
		if a.Changes() {
			t.Errorf("%s should not change the host", a)
		}
	}
}

// The order these are checked in matters. The most urgent thing a host has to say
// is the thing it should report.
func TestHostStateFromResources(t *testing.T) {
	cases := []struct {
		name string
		in   []Resource
		want Host
	}{
		{"nothing declared", nil, HostConverged},
		{"all converged", []Resource{Converged, Converged}, HostConverged},
		{"drift", []Resource{Converged, Drifted}, HostDrifted},
		{"a coverage gap", []Resource{Converged, Skipped}, Degraded},
		{"a failure", []Resource{Converged, Failed}, HostFailed},
		{"blocked counts as failed", []Resource{Converged, Blocked}, HostFailed},
		{"failure outweighs a gap", []Resource{Skipped, Failed}, HostFailed},
		{"a gap outweighs drift", []Resource{Drifted, Skipped}, Degraded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HostFrom(c.in); got != c.want {
				t.Errorf("HostFrom(%v) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

func TestStateNamesMatchTheDocumentation(t *testing.T) {
	resources := map[Resource]string{
		Pending: "pending", Applying: "applying", Converged: "converged",
		Drifted: "drifted", Failed: "failed", Blocked: "blocked", Skipped: "skipped",
	}
	for r, want := range resources {
		if got := r.String(); got != want {
			t.Errorf("resource state = %q, want %q", got, want)
		}
	}
	hosts := map[Host]string{
		Unknown: "unknown", HostConverged: "converged", HostDrifted: "drifted",
		HostFailed: "failed", Degraded: "degraded", AwaitingReboot: "awaiting-reboot",
	}
	for h, want := range hosts {
		if got := h.String(); got != want {
			t.Errorf("host state = %q, want %q", got, want)
		}
	}
	outcomes := map[Outcome]string{
		OutcomeConverged: "converged", OutcomeChanged: "changed", OutcomeFailed: "failed",
	}
	for o, want := range outcomes {
		if got := o.String(); got != want {
			t.Errorf("outcome = %q, want %q", got, want)
		}
	}
}
