// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"testing"

	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/provider/posix"
)

// TestMain fixes the capability set for every test in this package.
//
// The commands otherwise resolve capabilities from the machine running the tests, so the
// tests covering a type with no provider passed on a host without systemd and failed on one
// with it. The set here serves the filesystem types and nothing else, which is what those
// tests assert against.
func TestMain(m *testing.M) {
	detectCapabilities = func() (provider.Set, error) {
		set := provider.NewSet(posix.New())
		set.Host = "test"
		set.Unserved = map[string]string{
			"Package":    "",
			"Repository": "",
			"Service":    "",
			"User":       "",
			"Group":      "",
			"Sysctl":     "",
		}
		return set, nil
	}
	os.Exit(m.Run())
}
