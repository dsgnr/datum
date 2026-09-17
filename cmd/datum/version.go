// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/version"
)

func init() {
	register(command{
		name:    "version",
		summary: "Report the version, the revision it was built from, the platform and the schema versions read",
		run:     runVersion,
	})
}

func runVersion(e *env, args []string) int {
	if len(args) > 0 {
		e.errorf("usage: datum version\n")
		return exitError
	}
	e.printf("%s", version.Report())
	// Which schema versions a binary reads decides whether it can be pointed at a
	// given repository, so it belongs with the build information rather than being
	// something to infer from a parse failure.
	e.printf("schema    %s\n", document.SupportedSchemas())
	return exitOK
}
