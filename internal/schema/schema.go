// SPDX-License-Identifier: Apache-2.0

// Package schema checks desired state against the fields a resource type defines.
//
// The tables are the documentation's field reference as data. Adding a field is one
// line here plus whatever the provider does with it.
package schema

// kind is how a field's value is checked.
type kind int

const (
	text kind = iota // any safe string
	name             // a user, group, package or unit name
	enum             // one of a fixed set of values
	boolean
	integer
	absPath  // an absolute path on the host
	repoPath // a relative path inside the repository
	mode     // permission bits, written as quoted octal
	nameList // a list of names
	textList // a list of safe strings
)

type field struct {
	name     string
	kind     kind
	required bool
	values   []string // for enum
}

// resourceType is what is known about one type's desired state.
type resourceType struct {
	fields []field

	// atMostOne groups fields where declaring more than one is an error.
	atMostOne [][]string

	// requiredWhenPresent must be set unless the resource is declared absent.
	requiredWhenPresent []string
}

func (t resourceType) field(fieldName string) (field, bool) {
	for _, f := range t.fields {
		if f.name == fieldName {
			return f, true
		}
	}
	return field{}, false
}

// presence is the state field most types share.
var presence = field{name: "state", kind: enum, values: []string{"present", "absent"}}

var types = map[string]resourceType{
	"Package": {
		fields: []field{
			{name: "state", kind: enum, required: true, values: []string{"present", "absent"}},
			{name: "version", kind: text},
		},
	},

	"File": {
		fields: []field{
			{name: "path", kind: absPath, required: true},
			presence,
			{name: "content", kind: text},
			{name: "source", kind: repoPath},
			{name: "template", kind: repoPath},
			{name: "secretRef", kind: name},
			{name: "owner", kind: name},
			{name: "group", kind: name},
			{name: "mode", kind: mode},
			{name: "sensitive", kind: boolean},
			{name: "validate", kind: name},
			{name: "allowPrivileged", kind: boolean},
		},
		// Content comes from one place, or the provider would be choosing.
		atMostOne: [][]string{{"content", "source", "template", "secretRef"}},
	},

	"Directory": {
		fields: []field{
			{name: "path", kind: absPath, required: true},
			presence,
			{name: "owner", kind: name},
			{name: "group", kind: name},
			{name: "mode", kind: mode},
			{name: "allowPrivileged", kind: boolean},
		},
	},

	"Symlink": {
		fields: []field{
			{name: "path", kind: absPath, required: true},
			presence,
			{name: "target", kind: text},
			{name: "owner", kind: name},
			{name: "group", kind: name},
		},
		requiredWhenPresent: []string{"target"},
	},

	"Service": {
		fields: []field{
			{name: "state", kind: enum, required: true, values: []string{"running", "stopped"}},
			{name: "enabled", kind: boolean, required: true},
		},
	},

	"User": {
		fields: []field{
			{name: "state", kind: enum, required: true, values: []string{"present", "absent"}},
			{name: "uid", kind: integer},
			{name: "primaryGroup", kind: name},
			{name: "groups", kind: nameList},
			{name: "home", kind: absPath},
			{name: "shell", kind: absPath},
			{name: "comment", kind: text},
			{name: "system", kind: boolean},
			{name: "passwordRef", kind: name},
		},
	},

	"Group": {
		fields: []field{
			{name: "state", kind: enum, required: true, values: []string{"present", "absent"}},
			{name: "gid", kind: integer},
			{name: "system", kind: boolean},
		},
	},

	"Sysctl": {
		fields: []field{
			{name: "value", kind: text, required: true},
			presence,
		},
	},

	"Repository": {
		fields: []field{
			{name: "id", kind: name, required: true},
			presence,
			{name: "url", kind: text},
			{name: "signingKey", kind: text},
			{name: "unsigned", kind: boolean},
			{name: "enabled", kind: boolean},
			{name: "priority", kind: integer},
			{name: "suite", kind: name},
			{name: "components", kind: nameList},
		},
		requiredWhenPresent: []string{"url"},
	},
}

// Known reports whether a type has a field table. A type in
// document.ResourceTypes without one is a bug here, not a repository problem.
func Known(typeName string) bool {
	_, ok := types[typeName]
	return ok
}
