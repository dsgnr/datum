// SPDX-License-Identifier: Apache-2.0

package document

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ResourceTypes is every type the agent understands. An unrecognised type is an error,
// not a document to skip. Extensions will add to this once they exist.
var ResourceTypes = map[string]bool{
	"Package":    true,
	"File":       true,
	"Directory":  true,
	"Symlink":    true,
	"Service":    true,
	"User":       true,
	"Group":      true,
	"Sysctl":     true,
	"Repository": true,
}

// Field length limits. Neither needs to be unbounded.
const (
	MaxNameLength = 256
	MaxPathLength = 4096
)

// ValidName reports whether a name is safe everywhere Datum puts it.
//
// An allowed set, not a rejected one. A list of dangerous characters has to be complete
// to work, and never is.
func ValidName(s string) bool {
	if s == "" || len(s) > MaxNameLength {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_' || r == '+':
		default:
			return false
		}
	}
	return true
}

// ValidLabelKey allows one slash, so reserved keys such as datum/host fit.
func ValidLabelKey(s string) bool {
	prefix, rest, found := strings.Cut(s, "/")
	if !found {
		return ValidName(s)
	}
	return ValidName(prefix) && ValidName(rest)
}

// SafeText checks fields where ValidName would be too narrow, such as a symlink
// target. Direction overrides are refused because a value that renders one way and
// compares another defeats review.
func SafeText(s string) error {
	if !utf8.ValidString(s) {
		return errText("is not valid UTF-8")
	}
	for _, r := range s {
		if r == 0 {
			return errText("contains a NUL byte")
		}
		if unicode.IsControl(r) && r != '\t' && r != '\n' {
			return errText("contains a control character")
		}
		if isBidiControl(r) {
			return errText("contains a text direction override")
		}
	}
	return nil
}

// isBidiControl covers characters that reorder how text displays without changing
// what it compares as.
func isBidiControl(r rune) bool {
	switch r {
	case 0x061C, // arabic letter mark
		0x200E, 0x200F, // left-to-right and right-to-left mark
		0x202A, 0x202B, 0x202C, 0x202D, 0x202E, // embedding and override
		0x2066, 0x2067, 0x2068, 0x2069: // isolates
		return true
	}
	return false
}

type errText string

func (e errText) Error() string { return string(e) }

// ParseReference reads a resource reference written Type[name].
func ParseReference(s string) (Reference, bool) {
	open := strings.IndexByte(s, '[')
	if open <= 0 || !strings.HasSuffix(s, "]") {
		return Reference{}, false
	}
	typeName := s[:open]
	name := s[open+1 : len(s)-1]
	if !ResourceTypes[typeName] || !ValidName(name) {
		return Reference{}, false
	}
	return Reference{Type: typeName, Name: name}, true
}
