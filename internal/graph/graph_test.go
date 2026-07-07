// SPDX-License-Identifier: Apache-2.0

package graph

import (
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/resolve"
)

func ref(typeName, name string) document.Reference {
	return document.Reference{Type: typeName, Name: name}
}

// resource builds a manifest entry. fields become desired state, which matters
// for types whose target identity comes from there.
func resource(typeName, name string, fields map[string]string) resolve.Resource {
	desired := document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}
	for k, v := range fields {
		desired.Map[k] = document.Scalar(v)
	}
	return resolve.Resource{
		Ref:      ref(typeName, name),
		Desired:  desired,
		Position: document.Position{File: "base/resources.yaml"},
	}
}

func manifest(resources ...resolve.Resource) resolve.Manifest {
	return resolve.Manifest{Host: "web-001", Revision: "8b91f20", Resources: resources}
}

func orderOf(t *testing.T, g *Graph) []string {
	t.Helper()
	var out []string
	for _, r := range g.Order() {
		out = append(out, r.String())
	}
	return out
}

func TestTargetIdentityComesFromTheRightPlace(t *testing.T) {
	m := manifest(
		resource("Package", "nginx", nil),
		resource("File", "nginx-config", map[string]string{"path": "/etc/nginx/nginx.conf"}),
		resource("Repository", "internal", map[string]string{"id": "internal-mirror"}),
	)
	g, err := Build(m)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"Package[nginx]":       "nginx",
		"File[nginx-config]":   "/etc/nginx/nginx.conf",
		"Repository[internal]": "internal-mirror",
	}
	for refText, want := range cases {
		for r, node := range g.Nodes {
			if r.String() == refText && node.Target != want {
				t.Errorf("%s target = %q, want %q", refText, node.Target, want)
			}
		}
	}
}

func TestFileWithoutAPathIsAnError(t *testing.T) {
	_, err := Build(manifest(resource("File", "nginx-config", nil)))
	if err == nil || !strings.Contains(err.Error(), "desired.path") {
		t.Fatalf("err = %v, want a complaint about desired.path", err)
	}
}

func TestDuplicateTargetIsAnError(t *testing.T) {
	m := manifest(
		resource("File", "nginx-main", map[string]string{"path": "/etc/nginx/nginx.conf"}),
		resource("File", "nginx-copy", map[string]string{"path": "/etc/nginx/nginx.conf"}),
	)
	_, err := Build(m)
	if err == nil {
		t.Fatal("expected a duplicate target error")
	}
	for _, want := range []string{"File[nginx-copy]", "File[nginx-main]", "/etc/nginx/nginx.conf"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

// Two resources of different types at the same path are not a duplicate, because
// a Symlink and a File claiming one path is caught by each type's own rules
// rather than by target identity alone.
func TestSameTargetDifferentTypesIsNotADuplicate(t *testing.T) {
	m := manifest(
		resource("File", "a", map[string]string{"path": "/etc/thing"}),
		resource("Directory", "b", map[string]string{"path": "/etc/thing"}),
	)
	if _, err := Build(m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnresolvedReferenceIsAnError(t *testing.T) {
	r := resource("Service", "nginx", map[string]string{"state": "running"})
	r.Requires = []document.Reference{ref("Package", "nginx")}
	_, err := Build(manifest(r))
	if err == nil || !strings.Contains(err.Error(), "not in the manifest") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "web-001") {
		t.Errorf("error should name the host, got: %v", err)
	}
}

func TestSelfDependencyIsAnError(t *testing.T) {
	r := resource("Service", "nginx", map[string]string{"state": "running"})
	r.Requires = []document.Reference{ref("Service", "nginx")}
	_, err := Build(manifest(r))
	if err == nil || !strings.Contains(err.Error(), "depends on itself") {
		t.Fatalf("err = %v", err)
	}
}

func TestCycleIsAnError(t *testing.T) {
	svc := resource("Service", "nginx", map[string]string{"state": "running"})
	svc.Requires = []document.Reference{ref("File", "nginx-config")}

	file := resource("File", "nginx-config", map[string]string{"path": "/etc/nginx/nginx.conf"})
	file.Requires = []document.Reference{ref("Package", "nginx")}

	pkg := resource("Package", "nginx", nil)
	pkg.Requires = []document.Reference{ref("Service", "nginx")}

	_, err := Build(manifest(svc, file, pkg))
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("err = %v", err)
	}
}

func TestOrderRespectsDependencies(t *testing.T) {
	pkg := resource("Package", "nginx", nil)

	file := resource("File", "nginx-config", map[string]string{"path": "/etc/nginx/nginx.conf"})
	file.Requires = []document.Reference{ref("Package", "nginx")}

	svc := resource("Service", "nginx", map[string]string{"state": "running"})
	svc.Requires = []document.Reference{ref("Package", "nginx")}
	svc.RestartOn = []document.Reference{ref("File", "nginx-config")}

	g, err := Build(manifest(svc, file, pkg))
	if err != nil {
		t.Fatal(err)
	}

	order := orderOf(t, g)
	want := []string{"Package[nginx]", "File[nginx-config]", "Service[nginx]"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", order, want)
	}
}

// restartOn orders as well as triggering, so a reference in it does not also need
// to appear in requires.
func TestRestartOnOrders(t *testing.T) {
	file := resource("File", "nginx-config", map[string]string{"path": "/etc/nginx/nginx.conf"})
	svc := resource("Service", "nginx", map[string]string{"state": "running"})
	svc.RestartOn = []document.Reference{ref("File", "nginx-config")}

	g, err := Build(manifest(svc, file))
	if err != nil {
		t.Fatal(err)
	}
	order := orderOf(t, g)
	if order[0] != "File[nginx-config]" {
		t.Errorf("order = %v, want the file first", order)
	}
}

func TestReloadOnOrdersAndTriggers(t *testing.T) {
	file := resource("File", "nginx-config", map[string]string{"path": "/etc/nginx/nginx.conf"})
	svc := resource("Service", "nginx", map[string]string{"state": "running"})
	svc.ReloadOn = []document.Reference{ref("File", "nginx-config")}

	g, err := Build(manifest(svc, file))
	if err != nil {
		t.Fatal(err)
	}
	edges := g.Dependencies(ref("Service", "nginx"))
	if len(edges) != 1 || edges[0].Kind != ReloadOn || !edges[0].Kind.Triggers() {
		t.Fatalf("edges = %v", edges)
	}
	if orderOf(t, g)[0] != "File[nginx-config]" {
		t.Errorf("the file should be ordered first")
	}
}

func TestRequiresDoesNotTrigger(t *testing.T) {
	if Requires.Triggers() {
		t.Error("requires should order without triggering an update")
	}
}

// Plan order has to be the same on two runs over one manifest, so ties are broken
// by name rather than by map iteration order.
func TestOrderIsDeterministic(t *testing.T) {
	build := func() []string {
		var resources []resolve.Resource
		for _, name := range []string{"curl", "git", "vim", "htop", "jq"} {
			resources = append(resources, resource("Package", name, nil))
		}
		g, err := Build(manifest(resources...))
		if err != nil {
			t.Fatal(err)
		}
		return orderOf(t, g)
	}
	first := strings.Join(build(), ",")
	for i := 0; i < 20; i++ {
		if got := strings.Join(build(), ","); got != first {
			t.Fatalf("order changed between runs:\n%s\n%s", first, got)
		}
	}
	want := "Package[curl],Package[git],Package[htop],Package[jq],Package[vim]"
	if first != want {
		t.Errorf("order = %s, want %s", first, want)
	}
}

func TestDependentsFollowEdges(t *testing.T) {
	pkg := resource("Package", "nginx", nil)
	file := resource("File", "nginx-config", map[string]string{"path": "/etc/nginx/nginx.conf"})
	file.Requires = []document.Reference{ref("Package", "nginx")}

	g, err := Build(manifest(pkg, file))
	if err != nil {
		t.Fatal(err)
	}
	dependents := g.Dependents(ref("Package", "nginx"))
	if len(dependents) != 1 || dependents[0].To != ref("File", "nginx-config") {
		t.Fatalf("dependents = %v", dependents)
	}
}

func TestEmptyManifestBuilds(t *testing.T) {
	g, err := Build(manifest())
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Order()) != 0 {
		t.Errorf("order = %v, want empty", g.Order())
	}
}
