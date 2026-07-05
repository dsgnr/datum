// SPDX-License-Identifier: Apache-2.0

package document

import (
	"encoding/json"
	"sort"
	"strings"
)

// Kind is the shape of a Value.
type Kind int

const (
	KindScalar Kind = iota
	KindList
	KindMap
)

// Value is one node of a desired-state tree. Merging, canonical encoding and
// provenance all work on this shape, because nothing above the provider boundary
// knows what a field means.
type Value struct {
	Kind Kind

	// Kept as text so a quoted "0640" and a bare 0640 stay distinguishable until
	// field validation decides which the type wanted.
	Scalar string
	Quoted bool

	List []Value
	Map  map[string]Value

	// From is the layer that set this value. Maps do not carry it, because they
	// merge key by key and have no single source.
	From string

	// Displaced records values this one overrode, highest precedence first, for
	// datum explain.
	Displaced []Origin
}

// Origin is a value that lost a merge, kept so it can be reported.
type Origin struct {
	Layer  string
	Scalar string
}

func Scalar(text string) Value {
	return Value{Kind: KindScalar, Scalar: text}
}

func (v Value) IsZero() bool {
	return v.Kind == KindScalar && v.Scalar == "" && v.Map == nil && v.List == nil
}

// Lookup walks a dotted path such as "desired.path".
func (v Value) Lookup(path string) (Value, bool) {
	current := v
	for _, part := range strings.Split(path, ".") {
		if current.Kind != KindMap {
			return Value{}, false
		}
		next, ok := current.Map[part]
		if !ok {
			return Value{}, false
		}
		current = next
	}
	return current, true
}

// Keys returns the map keys sorted, so output does not depend on map ordering.
func (v Value) Keys() []string {
	keys := make([]string, 0, len(v.Map))
	for k := range v.Map {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Clone returns a deep copy. Merging must not alias layer values, or it would
// mutate a document that was already read.
func (v Value) Clone() Value {
	out := v
	out.Displaced = append([]Origin(nil), v.Displaced...)
	switch v.Kind {
	case KindList:
		out.List = make([]Value, len(v.List))
		for i, item := range v.List {
			out.List[i] = item.Clone()
		}
	case KindMap:
		out.Map = make(map[string]Value, len(v.Map))
		for k, field := range v.Map {
			out.Map[k] = field.Clone()
		}
	}
	return out
}

// Canonical writes the value deterministically, so identical desired state gives
// identical bytes whatever order the source was in.
//
// Provenance is left out on purpose, because moving a resource between layers would
// otherwise change the digest without changing what happens on the host.
func (v Value) Canonical(out *strings.Builder) {
	switch v.Kind {
	case KindScalar:
		writeJSONString(out, v.Scalar)
	case KindList:
		out.WriteByte('[')
		for i, item := range v.List {
			if i > 0 {
				out.WriteByte(',')
			}
			item.Canonical(out)
		}
		out.WriteByte(']')
	case KindMap:
		out.WriteByte('{')
		for i, key := range v.Keys() {
			if i > 0 {
				out.WriteByte(',')
			}
			writeJSONString(out, key)
			out.WriteByte(':')
			field := v.Map[key]
			field.Canonical(out)
		}
		out.WriteByte('}')
	}
}

func writeJSONString(out *strings.Builder, s string) {
	encoded, err := json.Marshal(s)
	if err != nil {
		// Only reachable for a value json cannot represent, which a string is not.
		panic("document: encoding a string failed: " + err.Error())
	}
	out.Write(encoded)
}
