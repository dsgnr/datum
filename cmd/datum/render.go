// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"
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

func runRender(args []string) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	host := fs.String("host", "", "resolve for a named host")
	repo := fs.String("repo", ".", "use a local checkout")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if *host == "" {
		fmt.Fprintln(os.Stderr, "datum render: --host is required")
		return exitError
	}

	result, revision, err := loadRepo(*repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return exitError
	}

	manifest, err := resolve.Host(result.Set, *host, revision)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return exitError
	}
	// A manifest with a cycle or a duplicate target is not applicable, so the
	// graph is part of resolving usefully.
	if _, err := graph.Build(manifest); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return exitError
	}

	printManifest(os.Stdout, manifest)
	return exitOK
}

func printManifest(out *os.File, m resolve.Manifest) {
	fmt.Fprintf(out, "host       %s\n", m.Host)
	fmt.Fprintf(out, "revision   %s\n", m.Revision)
	fmt.Fprintf(out, "manifest   %s\n", m.ShortDigest())
	fmt.Fprintf(out, "resources  %d\n\n", len(m.Resources))

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, resource := range m.Resources {
		fmt.Fprintf(w, "%s\t%s\n", resource.Ref, strings.Join(resource.Layers, ", "))
	}
	w.Flush()
}
