// SPDX-License-Identifier: Apache-2.0

// Package lock serialises passes against one host.
package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrHeld means another process is mid-pass.
var ErrHeld = errors.New("another pass is running on this host")

// Held describes who has the lock, so the error can say.
type Held struct {
	PID int
}

func (h Held) Error() string {
	return fmt.Sprintf("%v (pid %d)", ErrHeld, h.PID)
}

func (h Held) Unwrap() error { return ErrHeld }

// Lock is an acquired pass lock.
type Lock struct {
	file *os.File
}

// Acquire takes the pass lock in dir. With wait false it fails immediately if somebody
// else holds it, which is what an interactive command wants, because waiting silently
// is indistinguishable from hanging.
func Acquire(dir string, wait bool) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "pass.lock")

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	if err := flock(file, wait); err != nil {
		holder := readPID(file)
		file.Close()
		if errors.Is(err, ErrHeld) {
			return nil, Held{PID: holder}
		}
		return nil, err
	}

	// Recorded for the benefit of whoever is refused next, not read back here.
	if err := writePID(file); err != nil {
		unflock(file)
		file.Close()
		return nil, err
	}
	return &Lock{file: file}, nil
}

// Release drops the lock. The kernel would drop it when the process exits anyway,
// which is why there is no stale lock to clean up after a crash.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unflock(l.file)
	if closeErr := l.file.Close(); err == nil {
		err = closeErr
	}
	l.file = nil
	return err
}

func writePID(file *os.File) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	_, err := fmt.Fprintf(file, "%d\n", os.Getpid())
	return err
}

func readPID(file *os.File) int {
	if _, err := file.Seek(0, 0); err != nil {
		return 0
	}
	var pid int
	if _, err := fmt.Fscanf(file, "%d", &pid); err != nil {
		return 0
	}
	return pid
}
