//go:build unix

// SPDX-License-Identifier: Apache-2.0

package statedir

import (
	"io/fs"
	"syscall"
)

func ownerUID(info fs.FileInfo) (int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(stat.Uid), true
}
