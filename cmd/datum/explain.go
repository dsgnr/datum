// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/resolve"
)

func init() {
	register(command{
		name:    "explain",
		summary: "Show why a resource applies to a host and where its values came from",
		run:     runExplain,
	})
}

func runExplain(e *env, args []string) int {
	fs := newFlagSet(e, "explain")
	host := fs.String("host", "", "resolve for a named host")
	repo := fs.String("repo", ".", "use a local checkout")
	positional, parseErr := parseFlags(fs, args)
	if parseErr != nil {
		return exitError
	}
	if len(positional) != 1 {
		e.errorf("usage: datum explain Type[name] --host NAME\n")
		return exitError
	}
	if *host == "" {
		e.errorf("datum explain: --host is required\n")
		return exitError
	}

	wanted, ok := document.ParseReference(positional[0])
	if !ok {
		e.errorf("datum explain: %q is not a resource reference, write it as Type[name]\n", positional[0])
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

	resource, found := findResource(manifest, wanted)
	if !found {
		e.errorf("%s does not apply to %s\n", wanted, *host)
		return exitError
	}

	printExplanation(e.out, manifest, resource)
	return exitOK
}

func findResource(m resolve.Manifest, ref document.Reference) (resolve.Resource, bool) {
	for _, resource := range m.Resources {
		if resource.Ref == ref {
			return resource, true
		}
	}
	return resolve.Resource{}, false
}

func printExplanation(out io.Writer, m resolve.Manifest, r resolve.Resource) {
	target, err := document.TargetIdentity(r.Ref.Type, r.Ref.Name, r.Desired)
	if err != nil {
		target = "(no target identity)"
	}
	fmt.Fprintf(out, "%s   %s\n\n", r.Ref, target)

	contributed := map[string]bool{}
	for _, name := range r.Layers {
		contributed[name] = true
	}

	fmt.Fprintln(out, "contributed by")
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, layer := range m.Layers {
		if !contributed[layer.Name] {
			continue
		}
		fmt.Fprintf(w, "  %s\tprecedence %d\tmatched %s\n", layer.Name, layer.Precedence, reasonText(layer))
	}
	w.Flush()

	fmt.Fprintln(out, "\nfields")
	w = tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	printFields(w, "", r.Desired)
	w.Flush()

	printRefs(out, "requires", r.Requires)
	printRefs(out, "restartOn", r.RestartOn)
	printRefs(out, "reloadOn", r.ReloadOn)

	if len(r.Used) > 0 {
		fmt.Fprintln(out, "\nsubstitutions")
		w = tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, used := range r.Used {
			fmt.Fprintf(w, "  %s\t%s\n", used.Placeholder, used.Value)
		}
		w.Flush()
	}
}

func reasonText(layer resolve.MatchedLayer) string {
	if len(layer.Reasons) == 0 {
		return "every host"
	}
	return layer.Reasons.String()
}

// printFields reports where each field value came from and what it displaced.
func printFields(w *tabwriter.Writer, prefix string, v document.Value) {
	switch v.Kind {
	case document.KindMap:
		for _, key := range v.Keys() {
			printFields(w, joinPath(prefix, key), v.Map[key])
		}
	default:
		text := v.Scalar
		if v.Kind == document.KindList {
			var b strings.Builder
			v.Canonical(&b)
			text = b.String()
		}
		line := fmt.Sprintf("  %s\t%s\t%s", prefix, oneLine(text), v.From)
		if len(v.Displaced) > 0 {
			line += fmt.Sprintf("\toverrides %s from %s", oneLine(v.Displaced[0].Scalar), v.Displaced[0].Layer)
		}
		fmt.Fprintln(w, line)
	}
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// maxFieldWidth cuts a value short. File content is a field like any other and can
// be a whole configuration file.
const maxFieldWidth = 60

// oneLine makes a value safe to print in a column. A newline in the middle of a
// field would break the alignment of everything after it, so anything not
// printable on one line is quoted instead.
func oneLine(text string) string {
	if strings.ContainsAny(text, "\n\t\r") {
		text = strconv.Quote(text)
	}
	if len(text) > maxFieldWidth {
		return text[:maxFieldWidth-3] + "..."
	}
	return text
}

func printRefs(out io.Writer, name string, refs []document.Reference) {
	if len(refs) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s\n", name)
	for _, ref := range refs {
		fmt.Fprintf(out, "  %s\n", ref)
	}
}
