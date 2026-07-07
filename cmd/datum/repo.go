// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os/exec"
	"strings"

	"github.com/dsgnr/datum/internal/discover"
)

// loadRepo walks a checkout and returns its documents and revision.
func loadRepo(dir string) (discover.Result, string, error) {
	result, err := discover.Walk(dir)
	if err != nil {
		return result, "", err
	}
	return result, revisionOf(dir), nil
}

// revisionOf asks git which commit the checkout is on. A directory that is not a
// work tree still works, so this reports a placeholder rather than failing.
// Verifying the revision is a separate concern.
func revisionOf(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "(no revision)"
	}
	return strings.TrimSpace(string(out))
}
