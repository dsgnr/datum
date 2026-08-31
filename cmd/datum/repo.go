// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dsgnr/datum/internal/discover"
)

// checkout locates a fleet directory inside a git repository.
//
// The fleet does not have to be the repository root. In a monorepo it is usually a
// subdirectory, so anything that reads history has to know the difference between the
// two paths, because git works in repository terms while discovery works in fleet
// terms.
type checkout struct {
	// Dir is the fleet directory, as given on the command line.
	Dir string
	// Toplevel is the repository root, empty outside a work tree.
	Toplevel string
	// Prefix is Dir relative to Toplevel, empty when the fleet is the root.
	Prefix string
}

// inspect asks git where dir sits. A directory that is not a work tree is still usable,
// so this reports what it can rather than failing.
func inspect(dir string) checkout {
	out := checkout{Dir: dir}
	toplevel, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return out
	}
	out.Toplevel = toplevel
	// show-prefix is the path of the working directory below the root, with a
	// trailing slash, and empty at the root itself.
	prefix, err := gitOutput(dir, "rev-parse", "--show-prefix")
	if err != nil {
		return out
	}
	out.Prefix = strings.TrimSuffix(prefix, "/")
	return out
}

// Revision is the commit the checkout is on. Verifying it is a separate concern.
func (c checkout) Revision() string {
	if c.Toplevel == "" {
		return "(no revision)"
	}
	revision, err := gitOutput(c.Dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "(no revision)"
	}
	return revision
}

// fleetIn returns the fleet directory inside another checkout of the same repository,
// such as a worktree at an older revision.
func (c checkout) fleetIn(root string) string {
	if c.Prefix == "" {
		return root
	}
	return filepath.Join(root, filepath.FromSlash(c.Prefix))
}

// loadRepo walks a fleet directory and returns its documents and revision.
func loadRepo(dir string) (discover.Result, string, error) {
	result, err := discover.Walk(dir)
	if err != nil {
		return result, "", err
	}
	return result, inspect(dir).Revision(), nil
}

func gitOutput(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
