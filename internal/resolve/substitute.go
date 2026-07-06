// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dsgnr/datum/internal/document"
)

// Used records a placeholder a resource consumed, for datum explain.
type Used struct {
	// Placeholder is the text between the braces, such as "labels.site".
	Placeholder string
	Value       string
}

// substituter replaces placeholders with values from a Host document.
//
// Declared labels only. Secret placeholders are left for apply time, and anything else
// is an error instead of rendering empty.
type substituter struct {
	host   string
	labels map[string]string

	used map[string]string
	errs []string
}

func newSubstituter(host string, labels map[string]string) *substituter {
	return &substituter{host: host, labels: labels, used: map[string]string{}}
}

// Used returns the consumed placeholders in a stable order.
func (s *substituter) Used() []Used {
	keys := make([]string, 0, len(s.used))
	for k := range s.used {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]Used, 0, len(keys))
	for _, k := range keys {
		out = append(out, Used{Placeholder: k, Value: s.used[k]})
	}
	return out
}

func (s *substituter) Errors() []string { return s.errs }

// value substitutes every scalar in a tree.
func (s *substituter) value(v document.Value) document.Value {
	switch v.Kind {
	case document.KindScalar:
		out := v
		out.Scalar = s.text(v.Scalar)
		return out
	case document.KindList:
		out := v
		out.List = make([]document.Value, len(v.List))
		for i, item := range v.List {
			out.List[i] = s.value(item)
		}
		return out
	case document.KindMap:
		out := v
		out.Map = make(map[string]document.Value, len(v.Map))
		for key, field := range v.Map {
			out.Map[key] = s.value(field)
		}
		return out
	}
	return v
}

// text replaces the placeholders in one scalar. The result is not rescanned, so a
// label value containing braces is used literally.
func (s *substituter) text(in string) string {
	if !strings.Contains(in, "{{") {
		return in
	}

	var out strings.Builder
	rest := in
	for {
		open := strings.Index(rest, "{{")
		if open < 0 {
			out.WriteString(rest)
			return out.String()
		}
		close := strings.Index(rest[open:], "}}")
		if close < 0 {
			s.fail("unclosed {{ in %q", in)
			out.WriteString(rest)
			return out.String()
		}
		close += open

		out.WriteString(rest[:open])
		placeholder := strings.TrimSpace(rest[open+2 : close])
		out.WriteString(s.expand(placeholder, rest[open:close+2]))
		rest = rest[close+2:]
	}
}

// expand resolves one placeholder. literal is returned unchanged for the ones that
// are not ours.
func (s *substituter) expand(placeholder, literal string) string {
	switch {
	case placeholder == "host":
		s.used[placeholder] = s.host
		return s.host

	case strings.HasPrefix(placeholder, "labels."):
		key := strings.TrimPrefix(placeholder, "labels.")
		value, ok := s.labels[key]
		if !ok {
			s.fail("references labels.%s, which host %s does not declare", key, s.host)
			return ""
		}
		s.used[placeholder] = value
		return value

	case strings.HasPrefix(placeholder, "secrets."):
		// Resolved on the host at apply time, so it never affects the digest.
		return literal

	default:
		s.fail("unknown substitution {{ %s }}, only labels, host and secrets are available", placeholder)
		return ""
	}
}

func (s *substituter) fail(format string, args ...any) {
	s.errs = append(s.errs, fmt.Sprintf(format, args...))
}
