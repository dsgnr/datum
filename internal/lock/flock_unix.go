//go:build unix

// SPDX-License-Identifier: Apache-2.0

package lock

import (
	"os"
	"syscall"
)

func flock(file *os.File, wait bool) error {
	how := syscall.LOCK_EX
	if !wait {
		how |= syscall.LOCK_NB
	}
	err := syscall.Flock(int(file.Fd()), how)
	if err == syscall.EWOULDBLOCK {
		return ErrHeld
	}
	return err
}

func unflock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
