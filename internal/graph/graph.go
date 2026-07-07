// SPDX-License-Identifier: Apache-2.0

// Package graph validates an effective manifest and orders its resources.
//
// It runs before anything reads the host, so a cycle or a duplicate target is
// caught without touching a machine.
package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/resolve"
)

// EdgeKind is why one resource is ordered before another.
type EdgeKind int

const (
	// Requires orders and nothing else.
	Requires EdgeKind = iota
	// RestartOn orders, and a change to the dependency restarts the dependent.
	RestartOn
	// ReloadOn orders, and a change to the dependency reloads the dependent.
	ReloadOn
)

func (k EdgeKind) String() string {
	switch k {
	case RestartOn:
		return "restartOn"
	case ReloadOn:
		return "reloadOn"
	default:
		return "requires"
	}
}

// Triggers reports whether a change to the dependency updates the dependent.
func (k EdgeKind) Triggers() bool {
	return k == RestartOn || k == ReloadOn
}

// Edge points from a dependency to the resource that waits for it.
type Edge struct {
	From document.Reference
	To   document.Reference
	Kind EdgeKind
}

// Node is one resource in the graph, with the target it manages on the host.
type Node struct {
	Resource resolve.Resource
	// Target is what this resource manages, such as a path or a package name.
	Target string
}

// Graph is a validated manifest. Building one proves it is applicable at all.
type Graph struct {
	Nodes map[document.Reference]Node
	Edges []Edge

	// order is worked out once, at build time.
	order []document.Reference
}

// Build validates a manifest and returns its graph.
func Build(m resolve.Manifest) (*Graph, error) {
	g := &Graph{Nodes: make(map[document.Reference]Node, len(m.Resources))}
	var errs document.Errors

	for _, resource := range m.Resources {
		target, err := document.TargetIdentity(resource.Ref.Type, resource.Ref.Name, resource.Desired)
		if err != nil {
			errs.Add(resource.Position, "%s %v", resource.Ref, err)
			continue
		}
		g.Nodes[resource.Ref] = Node{Resource: resource, Target: target}
	}

	g.findDuplicateTargets(&errs)
	g.buildEdges(m, &errs)

	// A missing node would look like a satisfied dependency, so ordering waits
	// until every reference resolves.
	if errs.Len() == 0 {
		if err := g.sort(); err != nil {
			errs.Add(document.Position{}, "%v", err)
		}
	}

	if err := errs.Err(); err != nil {
		return nil, err
	}
	return g, nil
}

// findDuplicateTargets reports two resources managing the same thing. Applying
// both would mean they fight on every pass.
func (g *Graph) findDuplicateTargets(errs *document.Errors) {
	byTarget := map[string][]document.Reference{}
	for ref, node := range g.Nodes {
		key := ref.Type + " " + node.Target
		byTarget[key] = append(byTarget[key], ref)
	}

	var keys []string
	for key, refs := range byTarget {
		if len(refs) > 1 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	for _, key := range keys {
		refs := byTarget[key]
		sort.Slice(refs, func(i, j int) bool { return refs[i].String() < refs[j].String() })
		names := make([]string, 0, len(refs))
		for _, ref := range refs {
			names = append(names, ref.String())
		}
		target := g.Nodes[refs[0]].Target
		errs.Add(g.Nodes[refs[0]].Resource.Position,
			"%s both manage %s, so Datum cannot tell which one is right",
			strings.Join(names, " and "), target)
	}
}

func (g *Graph) buildEdges(m resolve.Manifest, errs *document.Errors) {
	for _, resource := range m.Resources {
		if _, ok := g.Nodes[resource.Ref]; !ok {
			// Already reported as having no target identity.
			continue
		}
		lists := []struct {
			kind EdgeKind
			refs []document.Reference
		}{
			{Requires, resource.Requires},
			{RestartOn, resource.RestartOn},
			{ReloadOn, resource.ReloadOn},
		}
		for _, list := range lists {
			for _, dependency := range list.refs {
				if _, ok := g.Nodes[dependency]; !ok {
					errs.Add(resource.Position,
						"%s %s %s, which is not in the manifest for %s",
						resource.Ref, list.kind, dependency, m.Host)
					continue
				}
				if dependency == resource.Ref {
					errs.Add(resource.Position, "%s depends on itself", resource.Ref)
					continue
				}
				g.Edges = append(g.Edges, Edge{From: dependency, To: resource.Ref, Kind: list.kind})
			}
		}
	}
	sort.Slice(g.Edges, func(i, j int) bool {
		a, b := g.Edges[i], g.Edges[j]
		if a.To != b.To {
			return a.To.String() < b.To.String()
		}
		return a.From.String() < b.From.String()
	})
}

// Dependencies returns the resources one resource waits for, in a stable order.
func (g *Graph) Dependencies(ref document.Reference) []Edge {
	var out []Edge
	for _, edge := range g.Edges {
		if edge.To == ref {
			out = append(out, edge)
		}
	}
	return out
}

// Dependents returns the resources waiting on one. Failure propagates along these.
func (g *Graph) Dependents(ref document.Reference) []Edge {
	var out []Edge
	for _, edge := range g.Edges {
		if edge.From == ref {
			out = append(out, edge)
		}
	}
	return out
}

// Order is the total order the planner builds a plan in.
func (g *Graph) Order() []document.Reference {
	return g.order
}

// sort produces a deterministic topological order. Ties break by reference name,
// so two runs over one manifest give the same plan.
func (g *Graph) sort() error {
	waitingFor := make(map[document.Reference]int, len(g.Nodes))
	for ref := range g.Nodes {
		waitingFor[ref] = 0
	}
	for _, edge := range g.Edges {
		waitingFor[edge.To]++
	}

	var ready []document.Reference
	for ref, count := range waitingFor {
		if count == 0 {
			ready = append(ready, ref)
		}
	}
	sortRefs(ready)

	order := make([]document.Reference, 0, len(g.Nodes))
	for len(ready) > 0 {
		next := ready[0]
		ready = ready[1:]
		order = append(order, next)

		var freed []document.Reference
		for _, edge := range g.Dependents(next) {
			waitingFor[edge.To]--
			if waitingFor[edge.To] == 0 {
				freed = append(freed, edge.To)
			}
		}
		if len(freed) > 0 {
			ready = append(ready, freed...)
			sortRefs(ready)
		}
	}

	if len(order) != len(g.Nodes) {
		return fmt.Errorf("dependency cycle\n  %s", strings.Join(g.describeCycle(waitingFor), " -> "))
	}
	g.order = order
	return nil
}

// describeCycle follows dependencies among the resources that never became ready until
// it revisits one, which gives a path an error can print.
func (g *Graph) describeCycle(waitingFor map[document.Reference]int) []string {
	var start document.Reference
	var stuck []document.Reference
	for ref, count := range waitingFor {
		if count > 0 {
			stuck = append(stuck, ref)
		}
	}
	if len(stuck) == 0 {
		return nil
	}
	sortRefs(stuck)
	start = stuck[0]

	inCycle := map[document.Reference]bool{}
	for _, ref := range stuck {
		inCycle[ref] = true
	}

	path := []string{start.String()}
	seen := map[document.Reference]bool{start: true}
	current := start
	for {
		var next document.Reference
		found := false
		for _, edge := range g.Dependencies(current) {
			if inCycle[edge.From] {
				next = edge.From
				found = true
				break
			}
		}
		if !found {
			return path
		}
		path = append(path, next.String())
		if seen[next] {
			return path
		}
		seen[next] = true
		current = next
	}
}

func sortRefs(refs []document.Reference) {
	sort.Slice(refs, func(i, j int) bool { return refs[i].String() < refs[j].String() })
}
