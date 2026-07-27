//go:build linux

// SPDX-License-Identifier: Apache-2.0

package systemd

import "os"

// booted reports whether systemd came up as PID 1, which is the directory it creates
// when it does. This is the same test sd_booted makes.
func booted() bool {
	info, err := os.Stat("/run/systemd/system")
	return err == nil && info.IsDir()
}
