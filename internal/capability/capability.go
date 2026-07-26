// SPDX-License-Identifier: Apache-2.0

// Package capability works out which resource types a host can reconcile. It is
// separate from package provider so that provider knows nothing of its own
// implementations.
package capability

import (
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/provider/apt"
	"github.com/dsgnr/datum/internal/provider/posix"
)

// Detect returns the capability set for this host.
//
// Selection is by what is installed rather than by what /etc/os-release claims, so
// a derivative nobody has heard of works without being listed anywhere.
//
// Providers for the types needing an init system or a user database are not written
// yet, so those types are absent and get skipped.
func Detect() provider.Set {
	providers := []provider.Provider{posix.New()}
	if apt.Detect() {
		providers = append(providers, apt.New())
	}
	return provider.NewSet(providers...)
}
