// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/dsgnr/datum/internal/document"
)

// Resource is what a caller hands over. Not the resolved type, so this package
// does not depend on the resolver.
type Resource struct {
	Ref      document.Reference
	Desired  document.Value
	Position document.Position
}

// Validate checks one resource's desired state against its type.
func Validate(r Resource, errs *document.Errors) {
	spec, ok := types[r.Ref.Type]
	if !ok {
		errs.Add(r.Position, "%s has no field table, which is a gap in Datum rather than in this repository", r.Ref)
		return
	}
	if r.Desired.Kind != document.KindMap {
		errs.Add(r.Position, "%s desired has to be a mapping of fields", r.Ref)
		return
	}

	for _, key := range r.Desired.Keys() {
		f, known := spec.field(key)
		if !known {
			errs.Add(r.Position, "%s has no field desired.%s", r.Ref, key)
			continue
		}
		checkValue(r, f, r.Desired.Map[key], errs)
	}

	for _, f := range spec.fields {
		if f.required && !set(r.Desired, f.name) {
			errs.Add(r.Position, "%s is missing desired.%s", r.Ref, f.name)
		}
	}

	for _, group := range spec.atMostOne {
		var declared []string
		for _, fieldName := range group {
			if set(r.Desired, fieldName) {
				declared = append(declared, "desired."+fieldName)
			}
		}
		if len(declared) > 1 {
			errs.Add(r.Position, "%s sets %s, which are mutually exclusive",
				r.Ref, strings.Join(declared, " and "))
		}
	}

	if present(r.Desired) {
		for _, fieldName := range spec.requiredWhenPresent {
			if !set(r.Desired, fieldName) {
				errs.Add(r.Position, "%s is missing desired.%s, which is required unless it is absent", r.Ref, fieldName)
			}
		}
	}

	checkTypeRules(r, errs)
}

// present reports whether a resource is declared to exist.
func present(desired document.Value) bool {
	value, ok := desired.Lookup("state")
	if !ok {
		return true
	}
	return value.Scalar != "absent"
}

func set(desired document.Value, fieldName string) bool {
	value, ok := desired.Lookup(fieldName)
	return ok && !(value.Kind == document.KindScalar && value.Scalar == "")
}

func checkValue(r Resource, f field, v document.Value, errs *document.Errors) {
	where := fmt.Sprintf("%s desired.%s", r.Ref, f.name)

	switch f.kind {
	case nameList, textList:
		if v.Kind != document.KindList {
			errs.Add(r.Position, "%s has to be a list", where)
			return
		}
		for _, item := range v.List {
			if item.Kind != document.KindScalar {
				errs.Add(r.Position, "%s entries have to be strings", where)
				continue
			}
			if f.kind == nameList && !document.ValidName(item.Scalar) {
				errs.Add(r.Position, "%s entry %q is not a valid name", where, item.Scalar)
			}
		}
		return
	}

	if v.Kind != document.KindScalar {
		errs.Add(r.Position, "%s has to be a single value", where)
		return
	}

	switch f.kind {
	case text:
		if err := document.SafeText(v.Scalar); err != nil {
			errs.Add(r.Position, "%s %v", where, err)
		}
	case name:
		if !document.ValidName(v.Scalar) {
			errs.Add(r.Position, "%s %q is not allowed, use letters, digits, dot, hyphen, underscore or plus", where, v.Scalar)
		}
	case enum:
		if !oneOf(f.values, v.Scalar) {
			errs.Add(r.Position, "%s is %q, which is not one of %s", where, v.Scalar, strings.Join(f.values, ", "))
		}
	case boolean:
		if v.Scalar != "true" && v.Scalar != "false" {
			errs.Add(r.Position, "%s has to be true or false", where)
		}
	case integer:
		if _, err := strconv.Atoi(v.Scalar); err != nil {
			errs.Add(r.Position, "%s has to be an integer", where)
		}
	case absPath:
		checkAbsPath(r, where, v.Scalar, errs)
	case repoPath:
		checkRepoPath(r, where, v.Scalar, errs)
	case mode:
		checkMode(r, where, v, errs)
	}
}

func checkAbsPath(r Resource, where, value string, errs *document.Errors) {
	switch {
	case value == "":
		errs.Add(r.Position, "%s is empty", where)
	case !strings.HasPrefix(value, "/"):
		errs.Add(r.Position, "%s has to be an absolute path", where)
	case len(value) > document.MaxPathLength:
		errs.Add(r.Position, "%s is longer than %d bytes", where, document.MaxPathLength)
	default:
		checkPathComponents(r, where, value, errs)
	}
}

// checkRepoPath covers source and template. Containment is checked when the file is
// read, and this catches the forms that can never be right.
func checkRepoPath(r Resource, where, value string, errs *document.Errors) {
	switch {
	case value == "":
		errs.Add(r.Position, "%s is empty", where)
	case strings.HasPrefix(value, "/"):
		errs.Add(r.Position, "%s has to be relative to the layer directory", where)
	default:
		checkPathComponents(r, where, value, errs)
		if cleaned := path.Clean(value); cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			errs.Add(r.Position, "%s resolves outside the layer directory", where)
		}
	}
}

func checkPathComponents(r Resource, where, value string, errs *document.Errors) {
	if err := document.SafeText(value); err != nil {
		errs.Add(r.Position, "%s %v", where, err)
		return
	}
	// Checked on the split, not the cleaned path, so a doubled slash shows.
	for _, component := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		switch component {
		case "":
			// Only the root path may have an empty component.
			if value != "/" {
				errs.Add(r.Position, "%s has an empty path component", where)
				return
			}
		case "..", ".":
			errs.Add(r.Position, "%s contains %q", where, component)
			return
		}
	}
}

// checkMode validates permission bits and refuses the ones that grant privilege
// without saying so. A mode has to be quoted, or its base depends on the YAML
// version.
func checkMode(r Resource, where string, v document.Value, errs *document.Errors) {
	if !v.Quoted {
		errs.Add(r.Position, "%s has to be quoted, so that %q is not read as a number", where, v.Scalar)
		return
	}
	digits := v.Scalar
	if len(digits) != 3 && len(digits) != 4 {
		errs.Add(r.Position, "%s is %q, which is not three or four octal digits", where, digits)
		return
	}
	bits, err := strconv.ParseInt(digits, 8, 32)
	if err != nil {
		errs.Add(r.Position, "%s is %q, which is not octal", where, digits)
		return
	}

	const (
		setuid = 0o4000
		setgid = 0o2000
		others = 0o002
	)
	privileged := bits&(setuid|setgid) != 0
	worldWritable := bits&others != 0

	if !privileged && !worldWritable {
		return
	}
	if allowed(r.Desired) {
		return
	}
	switch {
	case privileged:
		errs.Add(r.Position, "%s is %q, which sets the setuid or setgid bit, so it needs desired.allowPrivileged: true",
			where, digits)
	case worldWritable && systemPath(r.Desired):
		errs.Add(r.Position, "%s is %q, which is world writable under a system directory, so it needs desired.allowPrivileged: true",
			where, digits)
	}
}

func allowed(desired document.Value) bool {
	value, ok := desired.Lookup("allowPrivileged")
	return ok && value.Scalar == "true"
}

// systemPath reports where a world-writable file is an escalation and not merely
// untidy.
func systemPath(desired document.Value) bool {
	value, ok := desired.Lookup("path")
	if !ok {
		return false
	}
	for _, prefix := range []string{"/etc/", "/usr/", "/bin/", "/sbin/", "/lib/", "/boot/"} {
		if strings.HasPrefix(value.Scalar, prefix) {
			return true
		}
	}
	return false
}

func oneOf(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
