// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/osrelease"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/provider/providertest"
)

func fake(name string, types ...string) provider.Provider {
	return providertest.New(name, types...)
}

func debian() osrelease.Release { return osrelease.Release{ID: "debian"} }
func ubuntu() osrelease.Release { return osrelease.Release{ID: "ubuntu", Like: []string{"debian"}} }
func rocky() osrelease.Release {
	return osrelease.Release{ID: "rocky", Like: []string{"rhel", "centos", "fedora"}}
}
func unknown() osrelease.Release { return osrelease.Release{ID: "nixos"} }

func chosenFor(t *testing.T, set provider.Set, typeName string) string {
	t.Helper()
	p, ok := set.For(typeName)
	if !ok {
		t.Fatalf("nothing serves %s", typeName)
	}
	return p.Name()
}

func TestProviderClaimingTheHostIdentityWins(t *testing.T) {
	set, err := selectFrom([]candidate{
		{provider: fake("apt", "Package"), distributions: []string{"debian"}},
		{provider: fake("dnf", "Package"), distributions: []string{"fedora", "rhel"}},
	}, debian())
	if err != nil {
		t.Fatal(err)
	}
	if got := chosenFor(t, set, "Package"); got != "apt" {
		t.Errorf("chose %s, want apt", got)
	}
}

// ID_LIKE is what makes derivatives tractable without a list of every one of them.
func TestDerivativeFallsBackToIDLike(t *testing.T) {
	set, err := selectFrom([]candidate{
		{provider: fake("apt", "Package"), distributions: []string{"debian"}},
		{provider: fake("dnf", "Package"), distributions: []string{"fedora", "rhel"}},
	}, ubuntu())
	if err != nil {
		t.Fatal(err)
	}
	if got := chosenFor(t, set, "Package"); got != "apt" {
		t.Errorf("chose %s, want apt", got)
	}
}

// ID_LIKE is ordered closest first, so the first entry that matches decides.
func TestIDLikeIsTriedInOrder(t *testing.T) {
	set, err := selectFrom([]candidate{
		{provider: fake("rhelish", "Package"), distributions: []string{"rhel"}},
		{provider: fake("fedoraish", "Package"), distributions: []string{"fedora"}},
	}, rocky())
	if err != nil {
		t.Fatal(err)
	}
	if got := chosenFor(t, set, "Package"); got != "rhelish" {
		t.Errorf("chose %s, want rhelish, which rocky lists first", got)
	}
}

// The host's own ID beats anything it merely claims to resemble.
func TestIdentityBeatsIDLike(t *testing.T) {
	set, err := selectFrom([]candidate{
		{provider: fake("apt", "Package"), distributions: []string{"debian"}},
		{provider: fake("ubuntuish", "Package"), distributions: []string{"ubuntu"}},
	}, ubuntu())
	if err != nil {
		t.Fatal(err)
	}
	if got := chosenFor(t, set, "Package"); got != "ubuntuish" {
		t.Errorf("chose %s, want the provider naming ubuntu", got)
	}
}

// Choosing between two package managers is a decision for a person, so an even match is
// an error, not a preference.
func TestTwoProvidersAtTheSameSpecificityIsAnError(t *testing.T) {
	_, err := selectFrom([]candidate{
		{provider: fake("apt", "Package"), distributions: []string{"debian"}},
		{provider: fake("other", "Package"), distributions: []string{"debian"}},
	}, debian())
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"apt", "other", "debian"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

// Skipping rather than failing means a manifest partly supported on a host still does
// the rest of its work.
func TestNoProviderForATypeIsNotAnError(t *testing.T) {
	set, err := selectFrom([]candidate{
		{provider: fake("apt", "Package"), distributions: []string{"debian"}},
	}, unknown())
	if err != nil {
		t.Fatalf("err = %v, want nil so the resources are skipped", err)
	}
	if _, ok := set.For("Package"); ok {
		t.Error("nothing should serve Package on an unrecognised distribution")
	}
}

// A provider that names no distribution serves any host.
func TestProviderWithNoDistributionClaimServesAnything(t *testing.T) {
	for _, release := range []osrelease.Release{debian(), unknown(), {ID: "linux"}} {
		set, err := selectFrom([]candidate{
			{provider: fake("posix-file", "File")},
		}, release)
		if err != nil {
			t.Fatal(err)
		}
		if got := chosenFor(t, set, "File"); got != "posix-file" {
			t.Errorf("%s: chose %s", release.ID, got)
		}
	}
}

// A provider that named this distribution is more specific than one that named none.
func TestDistributionClaimBeatsNoClaim(t *testing.T) {
	set, err := selectFrom([]candidate{
		{provider: fake("generic", "Package")},
		{provider: fake("apt", "Package"), distributions: []string{"debian"}},
	}, debian())
	if err != nil {
		t.Fatal(err)
	}
	if got := chosenFor(t, set, "Package"); got != "apt" {
		t.Errorf("chose %s, want apt", got)
	}
}

// systemd is the case this exists for, serving any distribution and only where systemd
// is the init system.
func TestApplicableGatesAProvider(t *testing.T) {
	no := func() bool { return false }
	yes := func() bool { return true }

	set, err := selectFrom([]candidate{
		{provider: fake("systemd", "Service"), applicable: no},
	}, debian())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := set.For("Service"); ok {
		t.Error("a provider that is not applicable should not have been selected")
	}

	set, err = selectFrom([]candidate{
		{provider: fake("systemd", "Service"), applicable: yes},
	}, debian())
	if err != nil {
		t.Fatal(err)
	}
	if got := chosenFor(t, set, "Service"); got != "systemd" {
		t.Errorf("chose %s", got)
	}
}

// A provider ruled out by its own check cannot make a selection ambiguous.
func TestInapplicableProviderDoesNotCauseAmbiguity(t *testing.T) {
	no := func() bool { return false }

	set, err := selectFrom([]candidate{
		{provider: fake("apt", "Package"), distributions: []string{"debian"}},
		{provider: fake("other", "Package"), distributions: []string{"debian"}, applicable: no},
	}, debian())
	if err != nil {
		t.Fatal(err)
	}
	if got := chosenFor(t, set, "Package"); got != "apt" {
		t.Errorf("chose %s, want apt", got)
	}
}

// The set records the host in the terms selection used, so a skipped resource can say
// why, not only that it was.
func TestSetDescribesTheHost(t *testing.T) {
	set, err := selectFrom(nil, ubuntu())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ID=ubuntu", "ID_LIKE=debian"} {
		if !strings.Contains(set.Host, want) {
			t.Errorf("Host = %q, want it to contain %q", set.Host, want)
		}
	}

	set, err = selectFrom(nil, unknown())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(set.Host, "ID_LIKE unset") {
		t.Errorf("Host = %q", set.Host)
	}
}

// Selection does not probe for a provider's own programs, so a Debian host selects apt
// whether or not apt-get happens to be installed. A missing program is reported when
// the provider runs.
func TestSelectionDoesNotProbeForPrograms(t *testing.T) {
	set, err := selectFrom(candidates(), debian())
	if err != nil {
		t.Fatal(err)
	}
	if got := chosenFor(t, set, "Package"); got != "apt" {
		t.Errorf("chose %s, want apt on a Debian host", got)
	}
}

// The real candidate list has to be unambiguous on every distribution it claims, or
// no host of that kind could run a pass at all.
func TestRealCandidatesAreUnambiguous(t *testing.T) {
	for _, release := range []osrelease.Release{
		debian(), ubuntu(), rocky(), unknown(),
		{ID: "fedora"}, {ID: "alpine"}, {ID: "arch"}, {ID: "linux"},
	} {
		if _, err := selectFrom(candidates(), release); err != nil {
			t.Errorf("%s: %v", release.ID, err)
		}
	}
}
