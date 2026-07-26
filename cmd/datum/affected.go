// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/dsgnr/datum/internal/discover"
	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/resolve"
)

func init() {
	register(command{
		name:    "affected",
		summary: "Report which hosts a change between two revisions reaches",
		run:     runAffected,
	})
}

func runAffected(e *env, args []string) int {
	fs := newFlagSet(e, "affected")
	from := fs.String("from", "", "the revision to compare against")
	to := fs.String("to", "HEAD", "the revision to compare")
	repo := fs.String("repo", ".", "use a local checkout")
	host := fs.String("host", "", "expand one host into a per-resource diff")
	showResources := fs.Bool("show-resources", false, "show which resources changed")
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}
	if *from == "" {
		e.errorf("datum affected: --from is required\n")
		return exitError
	}

	before, err := resolveAt(*repo, *from)
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}
	after, err := resolveAt(*repo, *to)
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}

	if *showResources {
		if *host == "" {
			e.errorf("datum affected: --show-resources needs --host\n")
			return exitError
		}
		return printResourceDiff(e, *host, before, after)
	}

	printAffected(e.out, before, after)
	return exitOK
}

// snapshot is every host's digest at one revision.
type snapshot struct {
	revision  string
	digests   map[string]string
	manifests map[string]resolve.Manifest
	hosts     []string
}

// resolveAt resolves every host at one revision, using a worktree so that the caller's
// checkout is left alone.
func resolveAt(repo, revision string) (snapshot, error) {
	dir, err := os.MkdirTemp("", "datum-worktree-")
	if err != nil {
		return snapshot{}, err
	}
	defer os.RemoveAll(dir)

	tree := filepath.Join(dir, "tree")
	add := exec.Command("git", "-C", repo, "worktree", "add", "--quiet", "--detach", tree, revision)
	if out, err := add.CombinedOutput(); err != nil {
		return snapshot{}, fmt.Errorf("cannot read revision %s: %s", revision, strings.TrimSpace(string(out)))
	}
	defer func() {
		remove := exec.Command("git", "-C", repo, "worktree", "remove", "--force", tree)
		_ = remove.Run()
	}()

	result, err := discover.Walk(tree)
	if err != nil {
		return snapshot{}, fmt.Errorf("at revision %s: %w", revision, err)
	}
	return resolveAll(result, revision)
}

func resolveAll(result discover.Result, revision string) (snapshot, error) {
	out := snapshot{
		revision:  revision,
		digests:   map[string]string{},
		manifests: map[string]resolve.Manifest{},
	}
	for _, host := range result.Set.Hosts {
		manifest, err := resolve.Host(result.Set, host.Name, revision)
		if err != nil {
			return snapshot{}, fmt.Errorf("at revision %s, host %s:\n%v", revision, host.Name, err)
		}
		out.digests[host.Name] = manifest.ShortDigest()
		out.manifests[host.Name] = manifest
		out.hosts = append(out.hosts, host.Name)
	}
	sort.Strings(out.hosts)
	return out, nil
}

func printAffected(out io.Writer, before, after snapshot) {
	names := union(before.hosts, after.hosts)

	changed := 0
	for _, host := range names {
		if before.digests[host] != after.digests[host] {
			changed++
		}
	}
	fmt.Fprintf(out, "%d of %d hosts affected\n\n", changed, len(names))

	w := tabwriter.NewWriter(out, 0, 0, 4, ' ', 0)
	for _, host := range names {
		old, hadBefore := before.digests[host]
		new, hasAfter := after.digests[host]
		switch {
		case !hadBefore:
			fmt.Fprintf(w, "%s\tadded\t%s\n", host, new)
		case !hasAfter:
			fmt.Fprintf(w, "%s\tremoved\t%s\n", host, old)
		case old != new:
			fmt.Fprintf(w, "%s\t%s -> %s\n", host, old, new)
		default:
			fmt.Fprintf(w, "%s\tunchanged\n", host)
		}
	}
	w.Flush()
}

// printResourceDiff expands one host. This diffs two manifests, not two revisions, so
// it reports what the host's desired state becomes.
func printResourceDiff(e *env, host string, before, after snapshot) int {
	oldManifest, hadBefore := before.manifests[host]
	newManifest, hasAfter := after.manifests[host]
	if !hadBefore && !hasAfter {
		e.errorf("no Host document named %q at either revision\n", host)
		return exitError
	}

	e.printf("%s    %s -> %s\n\n", host, before.digests[host], after.digests[host])

	w := tabwriter.NewWriter(e.out, 0, 0, 4, ' ', 0)
	for _, line := range diffManifests(oldManifest, newManifest) {
		fmt.Fprintln(w, line)
	}
	w.Flush()
	return exitOK
}

func diffManifests(before, after resolve.Manifest) []string {
	oldResources := byRef(before)
	newResources := byRef(after)

	var refs []string
	seen := map[string]bool{}
	for ref := range oldResources {
		if !seen[ref] {
			refs = append(refs, ref)
			seen[ref] = true
		}
	}
	for ref := range newResources {
		if !seen[ref] {
			refs = append(refs, ref)
			seen[ref] = true
		}
	}
	sort.Strings(refs)

	var lines []string
	for _, ref := range refs {
		old, inOld := oldResources[ref]
		new, inNew := newResources[ref]
		switch {
		case !inOld:
			lines = append(lines, fmt.Sprintf("  %s\tadded\t%s", ref, strings.Join(new.Layers, ", ")))
		case !inNew:
			lines = append(lines, fmt.Sprintf("  %s\tremoved\t%s", ref, strings.Join(old.Layers, ", ")))
		default:
			lines = append(lines, diffFields(ref, old, new)...)
		}
	}
	return lines
}

func diffFields(ref string, before, after resolve.Resource) []string {
	oldFields := flatten("", before.Desired)
	newFields := flatten("", after.Desired)

	var names []string
	seen := map[string]bool{}
	for name := range oldFields {
		names = append(names, name)
		seen[name] = true
	}
	for name := range newFields {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var lines []string
	for _, name := range names {
		old, inOld := oldFields[name]
		new, inNew := newFields[name]
		switch {
		case !inOld:
			lines = append(lines, fmt.Sprintf("  %s\t%s\tset to %s", ref, name, new))
		case !inNew:
			lines = append(lines, fmt.Sprintf("  %s\t%s\tno longer set", ref, name))
		case old != new:
			lines = append(lines, fmt.Sprintf("  %s\t%s\t%s -> %s", ref, name, old, new))
		}
	}
	return lines
}

// flatten turns a desired-state tree into dotted field names, for comparing two
// manifests field by field.
func flatten(prefix string, v document.Value) map[string]string {
	out := map[string]string{}
	switch v.Kind {
	case document.KindMap:
		for _, key := range v.Keys() {
			for name, value := range flatten(joinPath(prefix, key), v.Map[key]) {
				out[name] = value
			}
		}
	default:
		var b strings.Builder
		v.Canonical(&b)
		text := b.String()
		if v.Kind == document.KindScalar {
			text = v.Scalar
		}
		out[prefix] = text
	}
	return out
}

func byRef(m resolve.Manifest) map[string]resolve.Resource {
	out := make(map[string]resolve.Resource, len(m.Resources))
	for _, resource := range m.Resources {
		out[resource.Ref.String()] = resource
	}
	return out
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]string{a, b} {
		for _, item := range list {
			if !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	sort.Strings(out)
	return out
}
