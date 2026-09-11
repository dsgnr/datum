// SPDX-License-Identifier: Apache-2.0

// Package version reports what this binary is.
//
// The version comes from an ldflag and the commit from the toolchain's own VCS stamping, so
// a build knows both without the Makefile passing the commit in. Both the command line and
// the metrics endpoint read them here, or a fleet could be told two different answers.
package version

import (
	"runtime"
	"runtime/debug"
)

// version is set at build time with -ldflags "-X .../internal/version.version=...". A build
// that does not set it says so, because a binary claiming a release number it was not built
// from is worse than one admitting it does not know.
var version = "unknown"

// Version is the release this binary was built as, or "unknown".
func Version() string { return version }

// Revision is the commit it was built from, empty when the build carried no VCS
// information. Modified reports whether the tree had uncommitted changes.
func Revision() (revision string, modified bool) {
	revision, modified, _ = stamp()
	return revision, modified
}

// Report is the text `datum version` prints. The revision is named as well as the version,
// because there are no releases yet and every build so far is a build of some commit.
func Report() string {
	out := "datum " + version + "\n"
	revision, modified, committed := stamp()
	if revision != "" {
		out += "revision  " + revision
		if committed != "" {
			out += ", committed " + committed
		}
		if modified {
			out += ", built with uncommitted changes"
		}
		out += "\n"
	}
	return out + "platform  " + runtime.GOOS + "/" + runtime.GOARCH + ", " + runtime.Version() + "\n"
}

// stamp reads what the toolchain recorded. The time is the commit's, not the build's.
func stamp() (revision string, modified bool, committed string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false, ""
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		case "vcs.time":
			committed = setting.Value
		}
	}
	return revision, modified, committed
}
