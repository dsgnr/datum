// SPDX-License-Identifier: Apache-2.0

package schema

import "github.com/dsgnr/datum/internal/document"

// checkTypeRules holds the rules that need more than one field, so cannot go in
// the table.
func checkTypeRules(r Resource, errs *document.Errors) {
	switch r.Ref.Type {
	case "Repository":
		checkRepository(r, errs)
	case "File":
		checkFile(r, errs)
	}
}

// checkRepository makes a trusted package source name its key. A missing key is
// invisible in review and the word "unsigned" is not.
func checkRepository(r Resource, errs *document.Errors) {
	if !present(r.Desired) {
		return
	}
	hasKey := set(r.Desired, "signingKey")
	unsigned := false
	if value, ok := r.Desired.Lookup("unsigned"); ok {
		unsigned = value.Scalar == "true"
	}

	switch {
	case !hasKey && !unsigned:
		errs.Add(r.Position, "%s has no desired.signingKey, so it needs desired.unsigned: true to say that is deliberate", r.Ref)
	case hasKey && unsigned:
		errs.Add(r.Position, "%s sets both desired.signingKey and desired.unsigned, which contradict each other", r.Ref)
	}
}

// checkFile rejects combinations that cannot mean anything, such as content on a
// file declared absent.
func checkFile(r Resource, errs *document.Errors) {
	if present(r.Desired) {
		return
	}
	for _, fieldName := range []string{"content", "source", "template", "secretRef", "owner", "group", "mode", "validate"} {
		if set(r.Desired, fieldName) {
			errs.Add(r.Position, "%s is absent but also sets desired.%s", r.Ref, fieldName)
		}
	}
}
