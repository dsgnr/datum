// SPDX-License-Identifier: Apache-2.0

package main

import (
	"runtime"
	"runtime/debug"
)

// version is set at build time with -ldflags "-X main.version=...". A build that does not
// set it says so, because a binary claiming a release number it was not built from is worse
// than one admitting it does not know.
var version = "unknown"

func init() {
	register(command{
		name:    "version",
		summary: "Report the version, the revision it was built from and the platform",
		run:     runVersion,
	})
}

func runVersion(e *env, args []string) int {
	if len(args) > 0 {
		e.errorf("usage: datum version\n")
		return exitError
	}
	e.printf("%s", versionReport())
	return exitOK
}

// versionReport names the revision as well as the version, because there are no releases
// yet and every build so far is a build of some commit.
func versionReport() string {
	report := "datum " + version + "\n"
	revision, modified, committed := vcs()
	if revision != "" {
		report += "revision  " + revision
		if committed != "" {
			report += ", committed " + committed
		}
		if modified {
			report += ", built with uncommitted changes"
		}
		report += "\n"
	}
	report += "platform  " + runtime.GOOS + "/" + runtime.GOARCH + ", " + runtime.Version() + "\n"
	return report
}

// vcs reads what the toolchain stamps into the binary, which is how a build knows its own
// commit without the Makefile passing one in. The time is the commit's, not the build's.
func vcs() (revision string, modified bool, committed string) {
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
