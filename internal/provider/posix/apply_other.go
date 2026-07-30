//go:build !linux

// SPDX-License-Identifier: Apache-2.0

package posix

import (
	"fmt"
	"runtime"

	"github.com/dsgnr/datum/internal/provider"
)

// Applying safely needs the descriptor-relative syscalls, and without them the rules
// could only be approximated. Reading is portable, so observe, diff and plan work here.

func write(req provider.Request) error { return unsupported() }

func remove(req provider.Request) error { return unsupported() }

func unsupported() error {
	return fmt.Errorf("posix-file applies changes on Linux only, this is %s", runtime.GOOS)
}
