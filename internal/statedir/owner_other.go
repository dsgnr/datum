//go:build !unix

// SPDX-License-Identifier: Apache-2.0

package statedir

import "io/fs"

func ownerUID(fs.FileInfo) (int, bool) { return 0, false }
