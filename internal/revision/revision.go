// SPDX-License-Identifier: Apache-2.0

// Package revision stores the one thing a pass carries to the next one.
//
// One value serves both downgrade protection, which requires a candidate to descend
// from it, and last known good, which selects what to reconcile when a newer revision
// fails to resolve.
//
// The pointer only advances. It moves to a revision that verified, satisfied the
// descendant requirement, resolved and validated. The outcome of applying does not move
// it.
package revision

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsgnr/datum/internal/statedir"
)

// Name is the file inside the state directory.
const Name = "accepted-revision"

// Path is where the accepted revision is recorded for a given state directory.
func Path(stateDir string) string {
	return filepath.Join(stateDir, Name)
}

// Read returns the accepted revision, and false when the host has none.
//
// No recorded revision means first contact, not an error. Whatever signed revision the
// host first sees becomes its baseline, which is a trust-on-first-use gap that
// provisioning closes by writing one.
func Read(stateDir string) (string, bool, error) {
	data, err := os.ReadFile(Path(stateDir))
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", false, nil
	}
	if err := validate(value); err != nil {
		// A malformed pointer is refused, never ignored. Treating it as absent would turn a
		// corrupted file into an accepted downgrade.
		return "", false, fmt.Errorf("%s holds %q, which is not a revision: %w", Path(stateDir), value, err)
	}
	return value, true, nil
}

// Advance records a newly accepted revision.
func Advance(stateDir, rev string) error {
	if err := validate(rev); err != nil {
		return err
	}
	if err := statedir.Ensure(stateDir); err != nil {
		return err
	}
	return writeAtomic(Path(stateDir), rev)
}

// Clear forgets the accepted revision, which is the documented recovery from a history
// that has been rewritten.
//
// A one-shot action and not a setting, because a control that can be switched off
// during an incident and left off is a suggestion. Clearing puts the host back to first
// contact, so the next signed revision it sees becomes its baseline.
func Clear(stateDir string) (string, bool, error) {
	previous, found, err := Read(stateDir)
	if err != nil {
		// Cleared anyway when the file is unreadable, since that is exactly the state
		// somebody is trying to recover from.
		previous, found = "", false
	}
	if err := os.Remove(Path(stateDir)); err != nil && !os.IsNotExist(err) {
		return "", false, err
	}
	return previous, found, nil
}

// validate refuses anything that is not a full object name.
//
// Abbreviated revisions are refused because an abbreviation that is unambiguous today
// can collide as a repository grows, and this value decides whether a revision is a
// downgrade.
func validate(rev string) error {
	if rev == "" {
		return fmt.Errorf("a revision cannot be empty")
	}
	// Both SHA-1 and SHA-256 object formats, since a repository may use either.
	if len(rev) != 40 && len(rev) != 64 {
		return fmt.Errorf("revision %q is not a full object name, which is %d or %d hex characters",
			rev, 40, 64)
	}
	for _, c := range rev {
		if !isHex(c) {
			return fmt.Errorf("revision %q contains %q, which is not hexadecimal", rev, c)
		}
	}
	return nil
}

func isHex(c rune) bool {
	switch {
	case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		return true
	default:
		return false
	}
}

// writeAtomic renames into place so a pass interrupted mid-write cannot leave a
// truncated pointer, which would read as a corrupted revision on the next pass.
func writeAtomic(path, value string) error {
	temp := path + ".tmp"
	if err := os.WriteFile(temp, []byte(value+"\n"), statedir.FileMode); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}
