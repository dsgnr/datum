// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/dsgnr/datum/internal/capability"
	"github.com/dsgnr/datum/internal/provider"
)

// detectCapabilities resolves the capability set for the host a command is running on.
//
// Replaced in tests, which otherwise depend on what the machine running them supports. A
// test asserting that a type with no provider is skipped passes on a host without systemd
// and fails on one with it.
var detectCapabilities = func() (provider.Set, error) { return capability.Detect() }
