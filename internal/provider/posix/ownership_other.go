//go:build !unix

// SPDX-License-Identifier: Apache-2.0

package posix

import (
	"fmt"
	"io/fs"
)

func ownership(fs.FileInfo) (string, string, error) {
	return "", "", fmt.Errorf("ownership is not available on this platform")
}
