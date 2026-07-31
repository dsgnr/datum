// SPDX-License-Identifier: Apache-2.0

// Package statedir guards the directory a pass keeps its records in.
//
// A plan names the files it would write and the fields it would change, so a state
// directory other local users can read leaks that. One they can write to lets them
// rewrite the accepted revision and pin the host at an older signed revision.
package statedir

import (
	"fmt"
	"io/fs"
	"os"
)

// Mode is the only permission set allowed on the state directory.
const Mode fs.FileMode = 0o700

// FileMode is the permission set for files inside it.
const FileMode fs.FileMode = 0o600

// Ensure creates the state directory if it is absent and checks it otherwise.
func Ensure(dir string) error {
	info, err := os.Stat(dir)
	switch {
	case os.IsNotExist(err):
		return os.MkdirAll(dir, Mode)
	case err != nil:
		return err
	}
	return check(dir, info)
}

// Check reports whether an existing state directory is safe to use.
func Check(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	return check(dir, info)
}

// Permissions are reported, never corrected. Silently widening or narrowing them would
// hide the fact that the directory had been open.
func check(dir string, info fs.FileInfo) error {
	if !info.IsDir() {
		return fmt.Errorf("state path %s is not a directory", dir)
	}
	if perm := info.Mode().Perm(); perm != Mode {
		return fmt.Errorf("state directory %s has mode %04o, want %04o", dir, perm, Mode)
	}
	// Ownership only matters when running as root, since an unprivileged pass
	// legitimately owns its own state directory.
	if os.Geteuid() != 0 {
		return nil
	}
	owner, ok := ownerUID(info)
	if !ok {
		return nil
	}
	if owner != 0 {
		return fmt.Errorf("state directory %s is owned by uid %d, want root", dir, owner)
	}
	return nil
}
