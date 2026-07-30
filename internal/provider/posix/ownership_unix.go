//go:build unix

// SPDX-License-Identifier: Apache-2.0

package posix

import (
	"fmt"
	"io/fs"
	"os/user"
	"strconv"
	"syscall"
)

// ownership reads the owning user and group as names, which is what a document
// declares. An id with no entry in the local database reads back as the number.
func ownership(info fs.FileInfo) (string, string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", fmt.Errorf("ownership is not available on this platform")
	}
	owner := strconv.FormatUint(uint64(stat.Uid), 10)
	if u, err := user.LookupId(owner); err == nil {
		owner = u.Username
	}
	group := strconv.FormatUint(uint64(stat.Gid), 10)
	if g, err := user.LookupGroupId(group); err == nil {
		group = g.Name
	}
	return owner, group, nil
}

func lookupOwner(name string) (int, error) {
	if u, err := user.Lookup(name); err == nil {
		return strconv.Atoi(u.Uid)
	}
	// A numeric id is accepted, because the account may not exist yet.
	if id, err := strconv.Atoi(name); err == nil {
		return id, nil
	}
	return -1, fmt.Errorf("no such user %q", name)
}

func lookupGroup(name string) (int, error) {
	if g, err := user.LookupGroup(name); err == nil {
		return strconv.Atoi(g.Gid)
	}
	if id, err := strconv.Atoi(name); err == nil {
		return id, nil
	}
	return -1, fmt.Errorf("no such group %q", name)
}
