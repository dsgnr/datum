// SPDX-License-Identifier: Apache-2.0

package plan

import (
	"sort"
	"strings"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/observe"
	"github.com/dsgnr/datum/internal/provider"
)

// FieldDiff is one field that does not hold the value the manifest asked for.
type FieldDiff struct {
	Field    string
	Desired  string
	Observed string
	// Missing reads better than an empty observed value.
	Missing bool
}

func (d FieldDiff) String() string {
	if d.Missing {
		return d.Field + " not set, want " + d.Desired
	}
	return d.Field + " " + d.Observed + " -> " + d.Desired
}

// Diff is the comparison of one resource's desired and observed state.
type Diff struct {
	Ref    document.Reference
	Fields []FieldDiff
	// Exists is whether the target is there at all.
	Exists bool
	// Occupied describes something of the wrong kind at the target.
	Occupied string
}

func (d Diff) Differs() bool {
	return len(d.Fields) > 0
}

// Compare works out which declared fields do not match what was read back. It is
// also how a pass verifies an action, by re-observing and comparing again.
//
// Only declared fields are compared, so a resource setting mode alone is not
// drifted by ownership however that looks on the host.
func Compare(ref document.Reference, desired document.Value, observation provider.Observation) Diff {
	out := Diff{Ref: ref, Exists: observation.Exists, Occupied: observation.Found}

	// The target identity field identifies the thing, it does not describe it.
	identity, _ := document.TargetIdentityField(ref.Type)

	for _, field := range declaredFields(desired) {
		// Fields that steer a provider have nothing on the host to compare.
		if field == identity || !comparable(field) {
			continue
		}
		if !observation.CanObserve(field) {
			continue
		}

		want := desired.Map[field]
		got, present := observation.Value(field)
		if !present {
			out.Fields = append(out.Fields, FieldDiff{
				Field:   field,
				Desired: render(want),
				Missing: true,
			})
			continue
		}
		if render(want) != render(got) {
			out.Fields = append(out.Fields, FieldDiff{
				Field:    field,
				Desired:  render(want),
				Observed: render(got),
			})
		}
	}
	return out
}

// notComparable tell a provider how to act, they do not describe a target.
var notComparable = map[string]bool{
	"state":           true,
	"source":          true,
	"template":        true,
	"secretRef":       true,
	"passwordRef":     true,
	"validate":        true,
	"allowPrivileged": true,
	"system":          true,
	"unsigned":        true,
	"suite":           true,
}

func comparable(field string) bool {
	return !notComparable[field]
}

func declaredFields(desired document.Value) []string {
	if desired.Kind != document.KindMap {
		return nil
	}
	out := desired.Keys()
	sort.Strings(out)
	return out
}

func render(v document.Value) string {
	if v.Kind == document.KindScalar {
		return v.Scalar
	}
	var b strings.Builder
	v.Canonical(&b)
	return b.String()
}

// diffAll compares every observed resource. A skipped or failed read produces no
// diff, having nothing to compare against.
func diffAll(observed observe.State, desired map[document.Reference]document.Value) map[document.Reference]Diff {
	out := make(map[document.Reference]Diff, len(observed.Results))
	for _, result := range observed.Results {
		if result.Skipped || result.Err != nil {
			continue
		}
		// A provider may normalise first, which is how content becomes a digest.
		want := result.Observation.DesiredOr(desired[result.Ref])
		out[result.Ref] = Compare(result.Ref, want, result.Observation)
	}
	return out
}
