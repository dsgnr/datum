// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFleet lays out a repository from a map of relative path to contents and
// returns the directory it was written to.
func writeFleet(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const fleetDoc = `
datum: v1alpha1
type: Fleet
name: example
`

const baseLayer = `
datum: v1alpha1
type: Layer
name: base
precedence: 0
`

func TestWalkFindsTheFleetRoot(t *testing.T) {
	dir := writeFleet(t, map[string]string{
		"fleet/datum.yaml":      fleetDoc,
		"fleet/base/layer.yaml": baseLayer,
	})

	result, err := Walk(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "fleet"); result.Root != want {
		t.Errorf("root = %q, want %q", result.Root, want)
	}
	if result.Set.Fleet.Name != "example" {
		t.Errorf("fleet name = %q", result.Set.Fleet.Name)
	}
}

func TestWalkNeedsAFleetDocument(t *testing.T) {
	dir := writeFleet(t, map[string]string{"fleet/base/layer.yaml": baseLayer})
	if _, err := Walk(dir); err == nil {
		t.Fatal("expected an error when no Fleet document exists")
	}
}

func TestWalkRejectsTwoFleetDocuments(t *testing.T) {
	dir := writeFleet(t, map[string]string{
		"a/datum.yaml": fleetDoc,
		"b/datum.yaml": fleetDoc,
	})
	_, err := Walk(dir)
	if err == nil || !strings.Contains(err.Error(), "expected one") {
		t.Fatalf("err = %v, want a complaint about two Fleet documents", err)
	}
}

// A resource belongs to the nearest Layer document at or above it, which is the
// only meaning a path carries.
func TestResourcesTakeTheNearestLayer(t *testing.T) {
	dir := writeFleet(t, map[string]string{
		"fleet/datum.yaml":      fleetDoc,
		"fleet/base/layer.yaml": baseLayer,
		"fleet/base/packages.yaml": `
datum: v1alpha1
type: Package
name: curl
desired:
  state: present
`,
		"fleet/roles/web/layer.yaml": `
datum: v1alpha1
type: Layer
name: role-web
precedence: 30
match:
  labels:
    role: web
`,
		"fleet/roles/web/deep/nested/nginx.yaml": `
datum: v1alpha1
type: Package
name: nginx
desired:
  state: present
`,
	})

	result, err := Walk(dir)
	if err != nil {
		t.Fatal(err)
	}

	layers := map[string]string{}
	for _, r := range result.Set.Resources {
		layers[r.Name] = r.Layer
	}
	if layers["curl"] != "base" {
		t.Errorf("curl is in layer %q, want base", layers["curl"])
	}
	if layers["nginx"] != "role-web" {
		t.Errorf("nginx is in layer %q, want role-web", layers["nginx"])
	}
}

func TestResourceWithNoLayerAboveItIsAnError(t *testing.T) {
	dir := writeFleet(t, map[string]string{
		"fleet/datum.yaml": fleetDoc,
		"fleet/orphan.yaml": `
datum: v1alpha1
type: Package
name: curl
desired:
  state: present
`,
	})
	_, err := Walk(dir)
	if err == nil || !strings.Contains(err.Error(), "no Layer document above it") {
		t.Fatalf("err = %v, want a complaint about a missing layer", err)
	}
}

func TestTwoLayersInOneDirectoryIsAnError(t *testing.T) {
	dir := writeFleet(t, map[string]string{
		"fleet/datum.yaml":      fleetDoc,
		"fleet/base/layer.yaml": baseLayer,
		"fleet/base/other.yaml": `
datum: v1alpha1
type: Layer
name: also-base
precedence: 5
`,
	})
	_, err := Walk(dir)
	if err == nil || !strings.Contains(err.Error(), "same directory") {
		t.Fatalf("err = %v, want a complaint about two layers in one directory", err)
	}
}

// Everything in one file is legal. The nearest-Layer rule is the only positional
// requirement, so a single file holding a whole fleet works.
func TestOneFileFleet(t *testing.T) {
	dir := writeFleet(t, map[string]string{
		"fleet/all.yaml": `
datum: v1alpha1
type: Fleet
name: example
---
datum: v1alpha1
type: Layer
name: base
---
datum: v1alpha1
type: Host
name: web-001
labels:
  role: web
---
datum: v1alpha1
type: Package
name: curl
desired:
  state: present
`,
	})

	result, err := Walk(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Set.Hosts) != 1 || len(result.Set.Layers) != 1 || len(result.Set.Resources) != 1 {
		t.Fatalf("hosts=%d layers=%d resources=%d",
			len(result.Set.Hosts), len(result.Set.Layers), len(result.Set.Resources))
	}
	if result.Set.Resources[0].Layer != "base" {
		t.Errorf("resource layer = %q, want base", result.Set.Resources[0].Layer)
	}
}

func TestExcludedFilesAreSkipped(t *testing.T) {
	dir := writeFleet(t, map[string]string{
		"fleet/datum.yaml": `
datum: v1alpha1
type: Fleet
name: example
exclude:
  - vendor
`,
		"fleet/base/layer.yaml": baseLayer,
		// Broken on purpose. It is excluded, so it must not be reported.
		"fleet/vendor/junk.yaml": "datum: v1alpha1\ntype: Nonsense\nname: x\n",
	})

	result, err := Walk(dir)
	if err != nil {
		t.Fatalf("excluded files should not be reported: %v", err)
	}
	if len(result.Set.Resources) != 0 {
		t.Errorf("got %d resources, want 0", len(result.Set.Resources))
	}
}

func TestFilesOutsideTheFleetRootAreIgnored(t *testing.T) {
	dir := writeFleet(t, map[string]string{
		"fleet/datum.yaml":      fleetDoc,
		"fleet/base/layer.yaml": baseLayer,
		// Outside the fleet root, so not fleet content whatever it holds.
		"zensical.yaml": "site_name: docs\n",
	})
	if _, err := Walk(dir); err != nil {
		t.Fatalf("files outside the fleet root should be ignored: %v", err)
	}
}

func TestErrorsUsePathsRelativeToTheFleetRoot(t *testing.T) {
	dir := writeFleet(t, map[string]string{
		"repo/fleet/datum.yaml":      fleetDoc,
		"repo/fleet/base/layer.yaml": baseLayer,
		"repo/fleet/base/bad.yaml": `
datum: v1alpha1
type: Package
name: curl
`,
	})
	_, err := Walk(dir)
	if err == nil {
		t.Fatal("expected an error for a resource with no desired")
	}
	if !strings.Contains(err.Error(), "base/bad.yaml") {
		t.Errorf("error should name the file relative to the fleet root, got: %v", err)
	}
}

func TestLayersAreSortedByPrecedenceThenPath(t *testing.T) {
	dir := writeFleet(t, map[string]string{
		"fleet/datum.yaml": fleetDoc,
		"fleet/b/layer.yaml": `
datum: v1alpha1
type: Layer
name: b-layer
precedence: 10
`,
		"fleet/a/layer.yaml": `
datum: v1alpha1
type: Layer
name: a-layer
precedence: 10
`,
		"fleet/base/layer.yaml": baseLayer,
	})

	result, err := Walk(dir)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, l := range result.Set.Layers {
		order = append(order, l.Name)
	}
	want := []string{"base", "a-layer", "b-layer"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"**/README.md", "README.md", true},
		{"**/README.md", "a/b/README.md", true},
		{"**/README.md", "a/README.txt", false},
		{"vendor", "vendor", true},
		{"vendor", "vendor/x.yaml", false},
		{"vendor/**", "vendor/x.yaml", true},
		{"*.yaml", "a.yaml", true},
		{"*.yaml", "a/b.yaml", false},
		{"a/*/c", "a/b/c", true},
		{"a/**/c", "a/c", true},
		{"a/**/c", "a/b/x/c", true},
	}
	for _, c := range cases {
		if got := matchGlob(c.pattern, c.path); got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestExcludedMatchesAncestorDirectories(t *testing.T) {
	if !excluded([]string{"vendor"}, "vendor/deep/file.yaml") {
		t.Error("a pattern matching a directory should exclude everything under it")
	}
	if excluded([]string{"vendor"}, "fleet/base/layer.yaml") {
		t.Error("unrelated paths should not be excluded")
	}
}
