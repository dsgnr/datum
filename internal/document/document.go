// SPDX-License-Identifier: Apache-2.0

package document

import "fmt"

// SchemaVersion is the only value the datum field may take. Anything else is refused,
// never guessed at.
const SchemaVersion = "v1alpha1"

// Document types that are part of the fleet model, not resource types.
const (
	TypeFleet = "Fleet"
	TypeHost  = "Host"
	TypeLayer = "Layer"
)

// ReservedLabelPrefix marks labels the resolver injects. A Host document setting
// one is an error, or it could claim to be a different host.
const ReservedLabelPrefix = "datum/"

// HostLabel is injected during resolution so a layer can match one named host.
const HostLabel = "datum/host"

// Position is where a document or field was read from, used in errors.
type Position struct {
	// File is relative to the fleet root.
	File string
	Line int
}

func (p Position) String() string {
	if p.Line == 0 {
		return p.File
	}
	return fmt.Sprintf("%s:%d", p.File, p.Line)
}

// Fleet marks the root of a fleet. There is one per repository.
type Fleet struct {
	Name     string
	Exclude  []string
	Position Position
}

// Host declares a machine and its classification. It has no desired state.
type Host struct {
	Name     string
	Labels   map[string]string
	Position Position
}

// Layer declares which hosts a set of resources applies to and how strongly.
type Layer struct {
	Name       string
	Precedence int
	Match      Matcher
	Position   Position

	// Dir holds the Layer document, relative to the fleet root. Resources at or
	// below it belong to this layer, and relative paths resolve against it.
	Dir string
}

// Resource is a typed description of one thing on a host.
type Resource struct {
	Type      string
	Name      string
	Requires  []Reference
	RestartOn []Reference
	ReloadOn  []Reference
	Desired   Value
	Position  Position

	// Layer is the nearest Layer document at or above this one.
	Layer string
	// LayerDir is that layer's directory, needed to resolve relative paths.
	LayerDir string
}

// Ref is how a resource is named inside Datum, written Type[name].
func (r Resource) Ref() Reference {
	return Reference{Type: r.Type, Name: r.Name}
}

// Reference names a resource in the same effective manifest.
type Reference struct {
	Type string
	Name string
}

func (r Reference) String() string {
	return r.Type + "[" + r.Name + "]"
}

// Matcher decides which hosts a layer applies to. Every form present has to hold.
// An empty matcher matches every host.
type Matcher struct {
	Labels  map[string]string
	OneOf   map[string][]string
	NoneOf  map[string][]string
	Has     []string
	Missing []string
}

func (m Matcher) IsEmpty() bool {
	return len(m.Labels) == 0 && len(m.OneOf) == 0 && len(m.NoneOf) == 0 &&
		len(m.Has) == 0 && len(m.Missing) == 0
}

// Set is everything discovery found in one repository.
type Set struct {
	Fleet     Fleet
	Hosts     []Host
	Layers    []Layer
	Resources []Resource
}
