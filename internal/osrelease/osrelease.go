// SPDX-License-Identifier: Apache-2.0

// Package osrelease identifies the distribution a host is running.
//
// This is the only thing above a provider that reads anything distribution specific,
// and it reads the minimum needed to say which provider serves this machine.
package osrelease

import (
	"bufio"
	"os"
	"strings"
)

// Paths are searched in order. The specification is explicit that the two files
// should not be merged, so the first one found is the one used.
var Paths = []string{"/etc/os-release", "/usr/lib/os-release"}

// Release is what selection needs from os-release.
type Release struct {
	// ID is the primary identifier, such as debian or fedora. The specification
	// says to assume linux when it is unset.
	ID string
	// Like is ID_LIKE, ordered closest first, and often empty.
	Like []string
	// Version is VERSION_ID, absent on rolling releases such as Arch. It can narrow
	// a choice and cannot be a precondition for making one.
	Version string
}

// Read identifies this host.
func Read() Release { return ReadFrom(Paths...) }

// ReadFrom identifies a host from the given files, taking the first that exists. A
// machine with no os-release at all is reported as linux with nothing else, which is
// what the specification says an unset ID means.
func ReadFrom(paths ...string) Release {
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		release := parse(file)
		file.Close()
		return release
	}
	return Release{ID: "linux"}
}

// Identifiers returns ID followed by ID_LIKE, which is the order selection tries them
// in, the host's own identity first and then what it claims to resemble.
func (r Release) Identifiers() []string {
	out := make([]string, 0, len(r.Like)+1)
	if r.ID != "" {
		out = append(out, r.ID)
	}
	return append(out, r.Like...)
}

func parse(file *os.File) Release {
	out := Release{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		value = unquote(value)
		switch key {
		case "ID":
			out.ID = value
		case "ID_LIKE":
			out.Like = strings.Fields(value)
		case "VERSION_ID":
			out.Version = value
		}
	}
	if out.ID == "" {
		out.ID = "linux"
	}
	return out
}

// unquote handles the shell-style quoting the format allows. Values are usually
// bare, and Ubuntu quotes ID_LIKE while Debian does not.
func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
