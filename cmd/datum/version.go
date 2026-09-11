// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/dsgnr/datum/internal/version"

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
	e.printf("%s", version.Report())
	return exitOK
}
