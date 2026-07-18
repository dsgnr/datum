//go:build !unix

// SPDX-License-Identifier: Apache-2.0

package lock

import (
	"fmt"
	"os"
	"runtime"
)

func flock(file *os.File, wait bool) error {
	return fmt.Errorf("the pass lock needs flock, which is not available on %s", runtime.GOOS)
}

func unflock(file *os.File) error { return nil }
