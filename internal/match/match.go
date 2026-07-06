// SPDX-License-Identifier: Apache-2.0

// Package match evaluates layer matchers against a host's labels.
package match

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dsgnr/datum/internal/document"
)

// Reason records why a layer matched. The resolver must not discard it, because
// datum explain is the reason the fleet model records provenance at all.
type Reason struct {
	Key   string
	Value string
}

func (r Reason) String() string {
	if r.Value == "" {
		return r.Key
	}
	return r.Key + "=" + r.Value
}

// Reasons is the set of label terms that caused a match, in a stable order.
type Reasons []Reason

func (rs Reasons) String() string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		parts = append(parts, r.String())
	}
	return strings.Join(parts, ", ")
}

// Evaluate reports whether a matcher selects a host, and which terms made it so.
// A matcher is one conjunction, and an empty one matches every host.
func Evaluate(m document.Matcher, labels map[string]string) (bool, Reasons) {
	if m.IsEmpty() {
		return true, nil
	}
	var reasons Reasons

	for _, key := range sortedKeys(m.Labels) {
		value, present := labels[key]
		if !present || value != m.Labels[key] {
			return false, nil
		}
		reasons = append(reasons, Reason{Key: key, Value: value})
	}

	for _, key := range sortedKeysOfLists(m.OneOf) {
		value, present := labels[key]
		if !present || !contains(m.OneOf[key], value) {
			return false, nil
		}
		reasons = append(reasons, Reason{Key: key, Value: value})
	}

	for _, key := range sortedKeysOfLists(m.NoneOf) {
		// Absent counts as not listed.
		if value, present := labels[key]; present && contains(m.NoneOf[key], value) {
			return false, nil
		}
	}

	for _, key := range sorted(m.Has) {
		value, present := labels[key]
		if !present {
			return false, nil
		}
		reasons = append(reasons, Reason{Key: key, Value: value})
	}

	for _, key := range sorted(m.Missing) {
		if _, present := labels[key]; present {
			return false, nil
		}
	}

	return true, reasons
}

// Validate reports matcher problems that hold regardless of any host.
func Validate(m document.Matcher, pos document.Position, errs *document.Errors) {
	for key, values := range m.OneOf {
		if len(values) == 0 {
			errs.Add(pos, "match.oneOf[%s] is empty, so it can never match", key)
		}
	}
	for key, values := range m.NoneOf {
		if len(values) == 0 {
			errs.Add(pos, "match.noneOf[%s] is empty, so it has no effect", key)
		}
	}
	// Worth naming, because the layer would silently never apply.
	missing := map[string]bool{}
	for _, key := range m.Missing {
		missing[key] = true
	}
	for _, key := range m.Has {
		if missing[key] {
			errs.Add(pos, "match requires %s to be both present and absent", key)
		}
	}
	for key := range m.Labels {
		if missing[key] {
			errs.Add(pos, "match requires %s to have a value and to be absent", key)
		}
	}
	for key := range m.OneOf {
		if missing[key] {
			errs.Add(pos, "match requires %s to have a value and to be absent", key)
		}
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeysOfLists(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Describe renders a matcher for error messages and command output.
func Describe(m document.Matcher) string {
	if m.IsEmpty() {
		return "every host"
	}
	var parts []string
	for _, key := range sortedKeys(m.Labels) {
		parts = append(parts, fmt.Sprintf("%s=%s", key, m.Labels[key]))
	}
	for _, key := range sortedKeysOfLists(m.OneOf) {
		parts = append(parts, fmt.Sprintf("%s in [%s]", key, strings.Join(m.OneOf[key], " ")))
	}
	for _, key := range sortedKeysOfLists(m.NoneOf) {
		parts = append(parts, fmt.Sprintf("%s not in [%s]", key, strings.Join(m.NoneOf[key], " ")))
	}
	for _, key := range sorted(m.Has) {
		parts = append(parts, "has "+key)
	}
	for _, key := range sorted(m.Missing) {
		parts = append(parts, "missing "+key)
	}
	return strings.Join(parts, ", ")
}
