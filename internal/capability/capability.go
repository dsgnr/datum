// SPDX-License-Identifier: Apache-2.0

// Package capability works out which resource types a host can reconcile, and which
// provider serves each one.
//
// It is separate from package provider so that provider knows nothing of its own
// implementations, and it is the only place that holds a list of them.
package capability

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dsgnr/datum/internal/osrelease"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/provider/apt"
	"github.com/dsgnr/datum/internal/provider/dnf"
	"github.com/dsgnr/datum/internal/provider/posix"
	"github.com/dsgnr/datum/internal/provider/procsys"
	"github.com/dsgnr/datum/internal/provider/systemd"
)

// candidate is one provider and the hosts it serves.
type candidate struct {
	provider provider.Provider

	// distributions are the os-release identifiers this provider claims. Empty
	// means the provider is not distribution specific.
	distributions []string

	// applicable answers whether the provider can work here at all, for the cases that are
	// not about which distribution this is. systemd is the example, serving any
	// distribution and only where systemd is the init system.
	//
	// This is not the same as probing for the provider's own programs. A provider chosen
	// for this distribution that then finds its package manager missing reports a failure,
	// which is more useful than quietly selecting a different one.
	applicable func() bool

	// needs names what applicable is checking for, so a skipped resource can say what is
	// missing rather than blaming the distribution.
	needs string
}

func (c candidate) usable() bool {
	return c.applicable == nil || c.applicable()
}

// candidates is every provider Datum knows how to build.
func candidates() []candidate {
	return []candidate{
		{provider: posix.New()},
		{provider: apt.New(), distributions: []string{"debian"}},
		{provider: dnf.New(), distributions: []string{"fedora", "rhel"}},
		{provider: procsys.New(), applicable: procsys.Detect, needs: "a writable /proc/sys"},
		{provider: systemd.New(), applicable: systemd.Detect, needs: "systemd as the init system"},
	}
}

// Detect returns the capability set for this host.
//
// Selection is by the host's os-release identity, not by probing for a package manager,
// because probing picks wrongly on the many machines that have more than one installed.
// A type with no provider here is skipped instead of failed.
func Detect() (provider.Set, error) {
	return selectFrom(candidates(), osrelease.Read())
}

// selectFrom applies the selection rule to a set of candidates.
//
// For each resource type the candidates are those claiming the host's ID. If none claim
// it, the ID_LIKE entries are tried in order, closest first, which is the order the
// specification says vendors list them. A provider that is not distribution specific is
// a candidate at every level and loses to one that named this distribution.
func selectFrom(all []candidate, release osrelease.Release) (provider.Set, error) {
	byType := map[string][]candidate{}
	for _, c := range all {
		if !c.usable() {
			continue
		}
		for _, typeName := range c.provider.Types() {
			byType[typeName] = append(byType[typeName], c)
		}
	}

	// Types that had a candidate ruled out by its own check, which is a different
	// gap from one nobody has written a provider for.
	ruledOut := map[string][]candidate{}
	for _, c := range all {
		if c.usable() {
			continue
		}
		for _, typeName := range c.provider.Types() {
			ruledOut[typeName] = append(ruledOut[typeName], c)
		}
	}

	var chosen []provider.Provider
	var problems []string
	unserved := map[string]string{}
	for _, typeName := range sorted(byType) {
		winner, err := pick(byType[typeName], release)
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("%s: %v", typeName, err))
		case winner != nil:
			chosen = append(chosen, winner)
			continue
		}
		// A type with no provider here is left out of the set, which makes its resources
		// skipped instead of failing the pass. A manifest partly supported on a host still
		// does the rest of its work.
		unserved[typeName] = missing(ruledOut[typeName])
	}
	for typeName, candidates := range ruledOut {
		if _, served := unserved[typeName]; served {
			continue
		}
		if _, ok := byType[typeName]; !ok {
			unserved[typeName] = missing(candidates)
		}
	}

	if len(problems) > 0 {
		return provider.Set{}, fmt.Errorf("provider selection is ambiguous on this host\n  %s",
			strings.Join(problems, "\n  "))
	}

	set := provider.NewSet(chosen...)
	set.Host = describe(release)
	if len(unserved) > 0 {
		set.Unserved = unserved
	}
	return set, nil
}

// missing describes what a ruled-out candidate was waiting for. An empty result means
// nothing was ruled out, so the distribution is the explanation.
func missing(candidates []candidate) string {
	var parts []string
	for _, c := range candidates {
		if c.needs != "" {
			parts = append(parts, c.provider.Name()+" needs "+c.needs)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// pick returns the one provider serving a type, nil when none serves it here, or an
// error when two are equally specific. Choosing between two package managers is a
// decision for a person, not a preference to encode.
func pick(candidates []candidate, release osrelease.Release) (provider.Provider, error) {
	for _, identifier := range release.Identifiers() {
		if winner, err := best(candidates, identifier); winner != nil || err != nil {
			return winner, err
		}
	}

	// Nothing named this distribution, so anything that is not distribution
	// specific serves it.
	var general []candidate
	for _, c := range candidates {
		if len(c.distributions) == 0 {
			general = append(general, c)
		}
	}
	switch len(general) {
	case 0:
		return nil, nil
	case 1:
		return general[0].provider, nil
	default:
		return nil, ambiguous(general, describe(release))
	}
}

// best returns the provider claiming one identifier, if exactly one does.
func best(candidates []candidate, identifier string) (provider.Provider, error) {
	var matched []candidate
	for _, c := range candidates {
		for _, claimed := range c.distributions {
			if claimed == identifier {
				matched = append(matched, c)
				break
			}
		}
	}
	switch len(matched) {
	case 0:
		return nil, nil
	case 1:
		return matched[0].provider, nil
	default:
		return nil, ambiguous(matched, identifier)
	}
}

func ambiguous(matched []candidate, scope string) error {
	names := make([]string, 0, len(matched))
	for _, c := range matched {
		names = append(names, c.provider.Name())
	}
	sort.Strings(names)
	return fmt.Errorf("%s both claim %s, so somebody has to choose",
		strings.Join(names, " and "), scope)
}

// describe names the host the way a plan should report it.
func describe(release osrelease.Release) string {
	out := "ID=" + release.ID
	if len(release.Like) == 0 {
		return out + ", ID_LIKE unset"
	}
	return out + ", ID_LIKE=" + strings.Join(release.Like, " ")
}

func sorted(m map[string][]candidate) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
