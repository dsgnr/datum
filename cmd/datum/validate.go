// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sort"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/graph"
	"github.com/dsgnr/datum/internal/match"
	"github.com/dsgnr/datum/internal/resolve"
)

func init() {
	register(command{
		name:    "validate",
		summary: "Parse every document and resolve every host",
		run:     runValidate,
	})
}

func runValidate(e *env, args []string) int {
	fs := newFlagSet(e, "validate")
	repo := fs.String("repo", ".", "use a local checkout")
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}

	result, revision, err := loadRepo(*repo)
	if err != nil {
		// Nothing resolves until the documents parse.
		e.errorf("%v\n", err)
		return exitError
	}

	e.printf("fleet      %s\n", result.Set.Fleet.Name)
	e.printf("revision   %s\n", revision)
	e.printf("hosts      %d\n", len(result.Set.Hosts))
	e.printf("layers     %d\n\n", len(result.Set.Layers))

	var errs errorGroups
	for _, layer := range result.Set.Layers {
		var layerErrs document.Errors
		match.Validate(layer.Match, layer.Position, &layerErrs)
		for _, e := range layerErrs.List() {
			errs.add(e.Error(), "")
		}
	}

	// Every host, because a conflict exists only for hosts both layers match and a
	// cycle can appear in one manifest and not another.
	for _, host := range result.Set.Hosts {
		manifest, err := resolve.Host(result.Set, host.Name, revision)
		if err != nil {
			errs.add(err.Error(), host.Name)
			continue
		}
		if _, err := graph.Build(manifest); err != nil {
			errs.add(err.Error(), host.Name)
		}
	}

	if errs.len() == 0 {
		e.printf("resolved %d hosts, 0 errors\n", len(result.Set.Hosts))
		return exitOK
	}

	for _, group := range errs.sorted() {
		e.errorf("error: %s\n", group.message)
		if group.count > 0 {
			e.errorf("  affects %s, first: %s\n", plural(group.count, "host"), group.first)
		}
		e.errorf("\n")
	}
	e.printf("resolved %d hosts, %s\n", len(result.Set.Hosts), plural(errs.len(), "error"))
	return exitError
}

// errorGroups collapses identical problems across hosts, so one typo in a widely
// matched layer is one error, not five hundred.
type errorGroups struct {
	order  []string
	groups map[string]*errorGroup
}

type errorGroup struct {
	message string
	first   string
	count   int
}

func (g *errorGroups) add(message, host string) {
	if g.groups == nil {
		g.groups = map[string]*errorGroup{}
	}
	existing, ok := g.groups[message]
	if !ok {
		existing = &errorGroup{message: message, first: host}
		g.groups[message] = existing
		g.order = append(g.order, message)
	}
	if host != "" {
		existing.count++
	}
}

func (g *errorGroups) len() int { return len(g.order) }

func (g *errorGroups) sorted() []errorGroup {
	out := make([]errorGroup, 0, len(g.order))
	for _, message := range g.order {
		out = append(out, *g.groups[message])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].message < out[j].message })
	return out
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
