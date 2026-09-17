// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sort"
	"strings"

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
	e.printf("layers     %d\n", len(result.Set.Layers))
	e.printf("schema     %s\n\n", schemaSummary(result.Set))

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
			// Split, so three mistakes count as three and each groups with the
			// other hosts sharing it.
			for _, message := range document.Split(err) {
				errs.add(message, host.Name)
			}
			continue
		}
		if _, err := graph.Build(manifest); err != nil {
			for _, message := range document.Split(err) {
				errs.add(message, host.Name)
			}
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

// schemaSummary counts the documents declaring each schema version.
//
// One version reads as a count. A repository part-way through a migration holds two, and
// listing both with their counts is what makes the remaining work visible, since each
// document is interpreted at the version it declares and nothing else reports the split.
func schemaSummary(set document.Set) string {
	counts := map[string]int{}
	add := func(version string) {
		if version != "" {
			counts[version]++
		}
	}
	add(set.Fleet.Schema)
	for _, host := range set.Hosts {
		add(host.Schema)
	}
	for _, layer := range set.Layers {
		add(layer.Schema)
	}
	for _, resource := range set.Resources {
		add(resource.Schema)
	}

	if len(counts) == 0 {
		return "none declared"
	}

	versions := make([]string, 0, len(counts))
	for version := range counts {
		versions = append(versions, version)
	}
	sort.Strings(versions)

	if len(versions) == 1 {
		return fmt.Sprintf("%s (%s)", versions[0], plural(counts[versions[0]], "document"))
	}
	parts := make([]string, 0, len(versions))
	for _, version := range versions {
		parts = append(parts, fmt.Sprintf("%s %d", version, counts[version]))
	}
	return strings.Join(parts, ", ") + ", migration in progress"
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
