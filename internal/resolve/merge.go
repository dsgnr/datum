// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"strings"

	"github.com/dsgnr/datum/internal/document"
)

// contribution is carried through a merge so a conflict can name the layers.
type contribution struct {
	layer      string
	precedence int
}

// conflict is two layers at equal precedence setting one field differently. Resolution
// fails instead of picking a winner the author cannot see.
type conflict struct {
	Field string
	A, B  contribution
	AText string
	BText string
}

// merger folds layer contributions together in ascending precedence order.
//
// Conflicts are per field, so the precedence compared is that of the layer which
// set that value. Two layers writing different fields do not conflict.
type merger struct {
	precedence map[string]int
	conflicts  []conflict
}

func newMerger(precedence map[string]int) *merger {
	return &merger{precedence: precedence}
}

// of returns the contribution recorded on a value. One with no recorded layer,
// which happens when a map is replaced by a scalar, counts as lower precedence.
func (m *merger) of(v document.Value) (contribution, bool) {
	if v.From == "" {
		return contribution{}, false
	}
	precedence, ok := m.precedence[v.From]
	if !ok {
		return contribution{layer: v.From}, false
	}
	return contribution{layer: v.From, precedence: precedence}, true
}

// merge combines an existing value with one from a layer of higher or equal precedence.
//
// Maps merge key by key. Everything else is replaced outright, which covers scalars and
// lists, and a list is replaced instead of appended, so a layer can shorten one.
func (m *merger) merge(field string, old, new document.Value, from contribution) document.Value {
	if old.Kind == document.KindMap && new.Kind == document.KindMap {
		out := old.Clone()
		if out.Map == nil {
			out.Map = map[string]document.Value{}
		}
		for _, key := range new.Keys() {
			child := new.Map[key]
			existing, present := out.Map[key]
			if !present {
				out.Map[key] = stamp(child, from)
				continue
			}
			out.Map[key] = m.merge(joinField(field, key), existing, child, from)
		}
		return out
	}

	if sameValue(old, new) {
		// Two layers agreeing is neither a conflict nor an override, so nothing
		// is displaced and the recorded source stays with the first one.
		return old
	}

	if previous, known := m.of(old); known && previous.precedence == from.precedence {
		m.conflicts = append(m.conflicts, conflict{
			Field: field,
			A:     previous,
			B:     from,
			AText: render(old),
			BText: render(new),
		})
		return old
	}

	out := stamp(new, from)
	displacedBy := old.From
	if displacedBy == "" {
		displacedBy = "an earlier layer"
	}
	out.Displaced = append([]document.Origin{{Layer: displacedBy, Scalar: render(old)}}, old.Displaced...)
	return out
}

// stamp records which layer contributed a value. Map nodes are stamped through to
// their children, because a map merges key by key and has no single source.
func stamp(v document.Value, from contribution) document.Value {
	out := v.Clone()
	if out.Kind == document.KindMap {
		for _, key := range out.Keys() {
			out.Map[key] = stamp(out.Map[key], from)
		}
		return out
	}
	out.From = from.layer
	return out
}

func sameValue(a, b document.Value) bool {
	return render(a) == render(b)
}

func render(v document.Value) string {
	if v.Kind == document.KindScalar {
		return v.Scalar
	}
	var b strings.Builder
	v.Canonical(&b)
	return b.String()
}

func joinField(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// mergeRefs combines reference lists as a set, keeping the order they were first
// seen so a plan is stable. A higher precedence layer cannot remove a dependency
// declared lower down, which is the point of treating these as sets.
func mergeRefs(existing, incoming []document.Reference) []document.Reference {
	seen := make(map[document.Reference]bool, len(existing)+len(incoming))
	out := make([]document.Reference, 0, len(existing)+len(incoming))
	for _, list := range [][]document.Reference{existing, incoming} {
		for _, ref := range list {
			if seen[ref] {
				continue
			}
			seen[ref] = true
			out = append(out, ref)
		}
	}
	return out
}
