// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/scaffold"
)

func init() {
	register(command{
		name:    "init",
		summary: "Create the documents a new repository needs",
		run:     runInit,
	})
}

func runInit(e *env, args []string) int {
	fs := newFlagSet(e, "init")
	repo := fs.String("repo", ".", "directory to create the repository in")
	name := fs.String("name", "example", "name for the Fleet document")
	withExamples := fs.Bool("with-examples", false, "also write a commented example host and resource")
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}

	if !document.ValidName(*name) {
		e.errorf("fleet name %q is not allowed, use letters, digits, dot, hyphen, underscore or plus\n", *name)
		return exitError
	}

	written, err := scaffold.Write(*repo, scaffold.Files(*name, *withExamples))
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}

	for _, path := range written {
		e.printf("created %s\n", path)
	}

	// Reported and not fatal. Desired state has to be committed before it can be
	// reconciled, and a repository that is never committed to has no effect on any
	// host, so a directory outside a work tree is worth saying out loud. Running
	// git init here is not the answer, because creating a repository inside another
	// one is a mess to unpick.
	if inspect(*repo).Toplevel == "" {
		e.errorf("\nwarning: %s is not inside a git work tree, and Datum reads desired state from git\n", *repo)
	}

	e.printf("\nnext steps\n")
	e.printf("  add a Host document under fleet/hosts/\n")
	e.printf("  add resources under fleet/base/ or a new layer\n")
	return exitOK
}
