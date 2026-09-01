// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dsgnr/datum/internal/document"
)

// CheckIdentity refuses a path that reaches protected state through the filesystem
// rather than through its name.
//
// The declared-path check cannot see these. A hard link at an innocent path shares an
// inode with the file it links to, so writing one writes the other and the two names
// have nothing in common. A symbolic link already on the host does the same by another
// route. Neither is visible anywhere but the host, which is why this runs before apply
// and not during validation.
//
// Anything that does not exist yet is not this check's business, since a path that is
// not there cannot already be a link to something.
func (s Set) CheckIdentity(ref document.Reference, target string) error {
	if !pathTypes[ref.Type] || target == "" || !filepath.IsAbs(target) {
		return nil
	}

	// Resolved with links followed, which is what a write to this path would actually
	// reach. A Symlink resource is exempt from this part, because writing a link replaces
	// the link itself instead of following it.
	if ref.Type != "Symlink" {
		if resolved, err := filepath.EvalSymlinks(target); err == nil && resolved != filepath.Clean(target) {
			if err := s.checkPath(ref, resolved); err != nil {
				return fmt.Errorf("%s resolves to protected state: %w", ref, err)
			}
		}
	}

	info, err := os.Lstat(target)
	if err != nil {
		// Absent, or unreadable for a reason apply will report better than this can.
		return nil
	}
	// A directory shares no inode with a file, and the containment cases are already
	// covered by name.
	if info.IsDir() {
		return nil
	}

	for _, protected := range s.protectedFiles() {
		same, err := sameFile(target, protected)
		if err != nil || !same {
			continue
		}
		return fmt.Errorf("%s targets %s, which is the same file as %s, and %s",
			ref, filepath.Clean(target), protected, protects(protected))
	}
	return nil
}

// protectedFiles is every exact file, plus what is currently inside a protected
// directory, since a hard link can point at a credential without naming its directory.
func (s Set) protectedFiles() []string {
	out := append([]string(nil), s.Files...)
	for _, dir := range s.Dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			out = append(out, filepath.Join(dir, entry.Name()))
		}
	}
	return out
}

// sameFile compares device and inode, not names, which is the only thing that answers
// whether two paths are one file.
func sameFile(a, b string) (bool, error) {
	first, err := os.Lstat(a)
	if err != nil {
		return false, err
	}
	second, err := os.Lstat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(first, second), nil
}
