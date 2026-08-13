// SPDX-License-Identifier: Apache-2.0

// Package observe reads the current state of the resources in a manifest. It
// changes nothing.
package observe

import (
	"context"
	"sort"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/graph"
	"github.com/dsgnr/datum/internal/provider"
)

// Result is one resource as it was found, or the reason it could not be read.
type Result struct {
	Ref document.Reference
	// Provider is empty when none supports the type.
	Provider    string
	Observation provider.Observation

	// Skipped means no provider here satisfies the type, which is a coverage gap, not a
	// failure. Reason says why.
	Skipped bool
	Reason  string
	// Err is set when a provider was asked and could not read.
	Err error
}

// State is the observed state of a whole manifest.
type State struct {
	Results []Result
	byRef   map[document.Reference]int
}

func (s State) For(ref document.Reference) (Result, bool) {
	index, ok := s.byRef[ref]
	if !ok {
		return Result{}, false
	}
	return s.Results[index], true
}

// Unsupported lists the resource types that were skipped, sorted.
func (s State) Unsupported() []string {
	seen := map[string]bool{}
	var out []string
	for _, result := range s.Results {
		if result.Skipped && !seen[result.Ref.Type] {
			seen[result.Ref.Type] = true
			out = append(out, result.Ref.Type)
		}
	}
	sort.Strings(out)
	return out
}

// skipReason says why nothing serves a type, preferring the explanation selection
// recorded over a guess from the distribution alone.
func skipReason(typeName string, providers provider.Set) string {
	if reason, ok := providers.Unserved[typeName]; ok && reason != "" {
		return "no " + typeName + " provider on this host: " + reason
	}
	if providers.Host == "" {
		return "no " + typeName + " provider on this host"
	}
	return "no " + typeName + " provider supports " + providers.Host
}

// Host reads every resource in a graph.
//
// Only the declared targets are read. A host with ten thousand packages and three
// declared ones does three reads.
func Host(ctx context.Context, g *graph.Graph, providers provider.Set, repoRoot string) (State, error) {
	out := State{byRef: map[document.Reference]int{}}

	// Observation order follows the plan order so that output is stable and
	// reading a report next to a plan lines up.
	for _, ref := range g.Order() {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		node := g.Nodes[ref]
		result := Result{Ref: ref}

		p, supported := providers.For(ref.Type)
		if !supported {
			result.Skipped = true
			result.Reason = skipReason(ref.Type, providers)
		} else {
			result.Provider = p.Name()
			observation, err := p.Observe(ctx, provider.Request{
				Ref:      ref,
				Target:   node.Target,
				Desired:  node.Resource.Desired,
				LayerDir: node.Resource.LayerDir,
				RepoRoot: repoRoot,
			})
			result.Observation = observation
			result.Err = err
		}

		out.byRef[ref] = len(out.Results)
		out.Results = append(out.Results, result)
	}
	return out, nil
}
