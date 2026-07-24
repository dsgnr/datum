// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/dsgnr/datum/internal/graph"
	"github.com/dsgnr/datum/internal/resolve"
)

func init() {
	register(command{
		name:    "render",
		summary: "Resolve desired state for a host and print it",
		run:     runRender,
	})
}

func runRender(e *env, args []string) int {
	fs := newFlagSet(e, "render")
	host := fs.String("host", "", "resolve for a named host")
	repo := fs.String("repo", ".", "use a local checkout")
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}
	if *host == "" {
		e.errorf("datum render: --host is required\n")
		return exitError
	}

	result, revision, err := loadRepo(*repo)
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}

	manifest, err := resolve.Host(result.Set, *host, revision)
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}
	// A manifest with a cycle or a duplicate target is not applicable, so the
	// graph is part of resolving usefully.
	if _, err := graph.Build(manifest); err != nil {
		e.errorf("%v\n", err)
		return exitError
	}

	printManifest(e.out, manifest)
	return exitOK
}

func printManifest(out io.Writer, m resolve.Manifest) {
	printHeader(out, m)

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, resource := range m.Resources {
		fmt.Fprintf(w, "%s\t%s\n", resource.Ref, strings.Join(resource.Layers, ", "))
	}
	w.Flush()
}

func printHeader(out io.Writer, m resolve.Manifest) {
	fmt.Fprintf(out, "host       %s\n", m.Host)
	fmt.Fprintf(out, "revision   %s\n", m.Revision)
	fmt.Fprintf(out, "manifest   %s\n", m.ShortDigest())
	fmt.Fprintf(out, "resources  %d\n\n", len(m.Resources))
}
