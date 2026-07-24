// SPDX-License-Identifier: Apache-2.0

// Package resolve turns a repository and a host name into an effective manifest.
//
// Nothing here reads the host, which is what lets a manifest be produced from a
// checkout and compared between revisions.
package resolve

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/match"
)

// Manifest is the resolved desired state for one host at one revision.
type Manifest struct {
	Host     string
	Revision string

	// Layers are the matching layers, in fold order.
	Layers []MatchedLayer
	// Resources are sorted by reference, so the manifest is stable.
	Resources []Resource
}

// MatchedLayer is a layer that applied, with the labels that caused it.
type MatchedLayer struct {
	Name       string
	Dir        string
	Precedence int
	Reasons    match.Reasons
}

// Resource is one merged resource, with enough provenance to answer why it applies.
type Resource struct {
	Ref       document.Reference
	Desired   document.Value
	Requires  []document.Reference
	RestartOn []document.Reference
	ReloadOn  []document.Reference

	// Layers contributed to this resource, lowest precedence first.
	Layers []string
	// LayerDir is where relative paths such as File.desired.source resolve from.
	LayerDir string
	// Used lists the placeholders substitution consumed.
	Used []Used

	Position document.Position
}

// Digest is the manifest's content address. Two hosts reporting the same one were
// given the same instructions.
func (m Manifest) Digest() string {
	var b strings.Builder
	for _, resource := range m.Resources {
		b.WriteString(resource.Ref.String())
		b.WriteByte(' ')
		resource.Desired.Canonical(&b)
		writeRefs(&b, "requires", resource.Requires)
		writeRefs(&b, "restartOn", resource.RestartOn)
		writeRefs(&b, "reloadOn", resource.ReloadOn)
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ShortDigest is what the commands print.
func (m Manifest) ShortDigest() string {
	full := m.Digest()
	const prefix = "sha256:"
	return prefix + strings.TrimPrefix(full, prefix)[:8]
}

func writeRefs(b *strings.Builder, name string, refs []document.Reference) {
	if len(refs) == 0 {
		return
	}
	// Sorted, or the digest would depend on which layer mentioned a dependency
	// first.
	sorted := append([]document.Reference(nil), refs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].String() < sorted[j].String() })

	b.WriteByte(' ')
	b.WriteString(name)
	b.WriteByte('[')
	for i, ref := range sorted {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(ref.String())
	}
	b.WriteByte(']')
}

// HostLabels returns the declared labels with the reserved ones injected.
func HostLabels(host document.Host) map[string]string {
	labels := make(map[string]string, len(host.Labels)+1)
	for k, v := range host.Labels {
		labels[k] = v
	}
	labels[document.HostLabel] = host.Name
	return labels
}

// Host resolves the effective manifest for one named host.
func Host(set document.Set, name, revision string) (Manifest, error) {
	host, ok := findHost(set, name)
	if !ok {
		return Manifest{}, fmt.Errorf("no Host document named %q in this fleet", name)
	}

	labels := HostLabels(host)
	manifest := Manifest{Host: name, Revision: revision}
	var errs document.Errors

	precedence := map[string]int{}
	for _, layer := range set.Layers {
		ok, reasons := match.Evaluate(layer.Match, labels)
		if !ok {
			continue
		}
		manifest.Layers = append(manifest.Layers, MatchedLayer{
			Name:       layer.Name,
			Dir:        layer.Dir,
			Precedence: layer.Precedence,
			Reasons:    reasons,
		})
		precedence[layer.Name] = layer.Precedence
	}

	// set.Layers is already in precedence then path order, so the fold is
	// deterministic.
	merged := map[document.Reference]*Resource{}
	var order []document.Reference
	m := newMerger(precedence)

	for _, layer := range manifest.Layers {
		for _, resource := range resourcesIn(set, layer.Name) {
			from := contribution{layer: layer.Name, precedence: layer.Precedence}

			sub := newSubstituter(name, labels)
			desired := sub.value(resource.Desired)
			for _, msg := range sub.Errors() {
				errs.Add(resource.Position, "%s.desired %s", resource.Ref(), msg)
			}

			existing, seen := merged[resource.Ref()]
			if !seen {
				merged[resource.Ref()] = &Resource{
					Ref:       resource.Ref(),
					Desired:   stamp(desired, from),
					Requires:  resource.Requires,
					RestartOn: resource.RestartOn,
					ReloadOn:  resource.ReloadOn,
					Layers:    []string{layer.Name},
					LayerDir:  resource.LayerDir,
					Used:      sub.Used(),
					Position:  resource.Position,
				}
				order = append(order, resource.Ref())
				continue
			}

			existing.Desired = m.merge(resource.Ref().String(), existing.Desired, desired, from)
			existing.Requires = mergeRefs(existing.Requires, resource.Requires)
			existing.RestartOn = mergeRefs(existing.RestartOn, resource.RestartOn)
			existing.ReloadOn = mergeRefs(existing.ReloadOn, resource.ReloadOn)
			existing.Layers = append(existing.Layers, layer.Name)
			existing.LayerDir = resource.LayerDir
			existing.Used = mergeUsed(existing.Used, sub.Used())
		}
	}

	for _, c := range m.conflicts {
		errs.Add(document.Position{}, "conflicting values for %s\n  %-6s %-22s precedence %d\n  %-6s %-22s precedence %d",
			c.Field,
			c.AText, c.A.layer, c.A.precedence,
			c.BText, c.B.layer, c.B.precedence)
	}

	for _, ref := range order {
		resource := merged[ref]
		if len(resource.RestartOn) > 0 && len(resource.ReloadOn) > 0 {
			errs.Add(resource.Position, "%s ends up with both restartOn and reloadOn after merging layers %s",
				ref, strings.Join(resource.Layers, ", "))
		}
		manifest.Resources = append(manifest.Resources, *resource)
	}
	sort.Slice(manifest.Resources, func(i, j int) bool {
		return manifest.Resources[i].Ref.String() < manifest.Resources[j].Ref.String()
	})

	if err := errs.Err(); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func findHost(set document.Set, name string) (document.Host, bool) {
	for _, host := range set.Hosts {
		if host.Name == name {
			return host, true
		}
	}
	return document.Host{}, false
}

func resourcesIn(set document.Set, layer string) []document.Resource {
	var out []document.Resource
	for _, resource := range set.Resources {
		if resource.Layer == layer {
			out = append(out, resource)
		}
	}
	return out
}

func mergeUsed(existing, incoming []Used) []Used {
	seen := make(map[string]bool, len(existing))
	for _, u := range existing {
		seen[u.Placeholder] = true
	}
	for _, u := range incoming {
		if !seen[u.Placeholder] {
			existing = append(existing, u)
			seen[u.Placeholder] = true
		}
	}
	sort.Slice(existing, func(i, j int) bool { return existing[i].Placeholder < existing[j].Placeholder })
	return existing
}
