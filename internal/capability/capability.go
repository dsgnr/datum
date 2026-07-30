// SPDX-License-Identifier: Apache-2.0

// Package capability works out which resource types a host can reconcile. It is
// separate from package provider so that provider knows nothing of its own
// implementations.
package capability

import (
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/provider/posix"
)

// Detect returns the capability set for this host.
//
// Providers for the types needing a package manager, an init system or a user
// database are not written yet, so those types are absent and get skipped.
func Detect() provider.Set {
	return provider.NewSet(posix.New())
}
