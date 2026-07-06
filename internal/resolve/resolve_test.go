// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
)

// builder assembles a document set without going through the filesystem, so the
// resolver tests stay about resolution.
type builder struct {
	set document.Set
}

func (b *builder) host(name string, labels map[string]string) *builder {
	b.set.Hosts = append(b.set.Hosts, document.Host{Name: name, Labels: labels})
	return b
}

func (b *builder) layer(name string, precedence int, m document.Matcher) *builder {
	b.set.Layers = append(b.set.Layers, document.Layer{
		Name:       name,
		Precedence: precedence,
		Match:      m,
		Dir:        name,
		Position:   document.Position{File: name + "/layer.yaml"},
	})
	return b
}

// resource adds a resource to a layer. Fields are given as a flat map for
// brevity, which covers every case these tests need.
func (b *builder) resource(layer, typeName, name string, fields map[string]string) *builder {
	desired := document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}
	for k, v := range fields {
		desired.Map[k] = document.Scalar(v)
	}
	b.set.Resources = append(b.set.Resources, document.Resource{
		Type:     typeName,
		Name:     name,
		Desired:  desired,
		Layer:    layer,
		LayerDir: layer,
		Position: document.Position{File: layer + "/resources.yaml"},
	})
	return b
}

func (b *builder) resolve(t *testing.T, host string) Manifest {
	t.Helper()
	m, err := Host(b.set, host, "8b91f20")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	return m
}

func labelsOf(role string) map[string]string {
	return map[string]string{"role": role, "environment": "production", "site": "london"}
}

func matchRole(role string) document.Matcher {
	return document.Matcher{Labels: map[string]string{"role": role}}
}

func field(t *testing.T, m Manifest, ref, path string) document.Value {
	t.Helper()
	for _, r := range m.Resources {
		if r.Ref.String() == ref {
			v, ok := r.Desired.Lookup(path)
			if !ok {
				t.Fatalf("%s has no %s", ref, path)
			}
			return v
		}
	}
	t.Fatalf("%s is not in the manifest", ref)
	return document.Value{}
}

func TestResolveOnlyIncludesMatchingLayers(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("base", 0, document.Matcher{}).
		layer("roles/web", 30, matchRole("web")).
		layer("roles/db", 30, matchRole("database")).
		resource("base", "Package", "curl", map[string]string{"state": "present"}).
		resource("roles/web", "Package", "nginx", map[string]string{"state": "present"}).
		resource("roles/db", "Package", "postgresql", map[string]string{"state": "present"})

	m := b.resolve(t, "web-001")

	var refs []string
	for _, r := range m.Resources {
		refs = append(refs, r.Ref.String())
	}
	want := "Package[curl],Package[nginx]"
	if got := strings.Join(refs, ","); got != want {
		t.Errorf("resources = %s, want %s", got, want)
	}
}

func TestHigherPrecedenceWins(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("roles/web", 30, matchRole("web")).
		layer("hosts/web-001", 100, document.Matcher{Labels: map[string]string{document.HostLabel: "web-001"}}).
		resource("roles/web", "File", "nginx-config", map[string]string{
			"path": "/etc/nginx/nginx.conf", "mode": "0640", "owner": "root",
		}).
		resource("hosts/web-001", "File", "nginx-config", map[string]string{"mode": "0600"})

	m := b.resolve(t, "web-001")

	mode := field(t, m, "File[nginx-config]", "mode")
	if mode.Scalar != "0600" {
		t.Errorf("mode = %q, want 0600", mode.Scalar)
	}
	if mode.From != "hosts/web-001" {
		t.Errorf("mode came from %q, want hosts/web-001", mode.From)
	}
	if len(mode.Displaced) != 1 || mode.Displaced[0].Scalar != "0640" {
		t.Errorf("displaced = %v, want 0640 from roles/web", mode.Displaced)
	}

	// Fields the higher layer did not mention keep their original source.
	owner := field(t, m, "File[nginx-config]", "owner")
	if owner.Scalar != "root" || owner.From != "roles/web" {
		t.Errorf("owner = %q from %q", owner.Scalar, owner.From)
	}
}

func TestEqualPrecedenceConflictIsAnError(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("roles/web", 30, matchRole("web")).
		layer("roles/web-tls", 30, matchRole("web")).
		resource("roles/web", "File", "nginx-config", map[string]string{"mode": "0640"}).
		resource("roles/web-tls", "File", "nginx-config", map[string]string{"mode": "0600"})

	_, err := Host(b.set, "web-001", "8b91f20")
	if err == nil {
		t.Fatal("expected a conflict")
	}
	for _, want := range []string{"conflicting values", "File[nginx-config].mode", "0640", "0600", "roles/web", "roles/web-tls"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got:\n%s", want, err)
		}
	}
}

func TestEqualPrecedenceSameValueIsNotAConflict(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("roles/web", 30, matchRole("web")).
		layer("roles/web-tls", 30, matchRole("web")).
		resource("roles/web", "File", "nginx-config", map[string]string{"mode": "0640"}).
		resource("roles/web-tls", "File", "nginx-config", map[string]string{"mode": "0640"})

	m := b.resolve(t, "web-001")
	if got := field(t, m, "File[nginx-config]", "mode").Scalar; got != "0640" {
		t.Errorf("mode = %q", got)
	}
}

// Two layers at the same precedence writing different fields is not a conflict.
// Getting this wrong means comparing against whichever layer touched the resource
// last rather than the one that set the field.
func TestEqualPrecedenceDifferentFieldsIsNotAConflict(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("base", 0, document.Matcher{}).
		layer("roles/web", 30, matchRole("web")).
		layer("roles/web-tls", 30, matchRole("web")).
		resource("base", "File", "nginx-config", map[string]string{"mode": "0644"}).
		resource("roles/web", "File", "nginx-config", map[string]string{"owner": "root"}).
		resource("roles/web-tls", "File", "nginx-config", map[string]string{"mode": "0600"})

	m := b.resolve(t, "web-001")
	if got := field(t, m, "File[nginx-config]", "mode").Scalar; got != "0600" {
		t.Errorf("mode = %q, want 0600", got)
	}
	if got := field(t, m, "File[nginx-config]", "owner").Scalar; got != "root" {
		t.Errorf("owner = %q", got)
	}
}

func TestListsAreReplacedNotAppended(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("base", 0, document.Matcher{}).
		layer("roles/web", 30, matchRole("web"))

	list := func(items ...string) document.Value {
		out := document.Value{Kind: document.KindList}
		for _, item := range items {
			out.List = append(out.List, document.Scalar(item))
		}
		return out
	}
	add := func(layer string, values document.Value) {
		b.set.Resources = append(b.set.Resources, document.Resource{
			Type: "Repository", Name: "internal", Layer: layer, LayerDir: layer,
			Desired: document.Value{Kind: document.KindMap, Map: map[string]document.Value{
				"components": values,
			}},
		})
	}
	add("base", list("main", "contrib"))
	add("roles/web", list("main"))

	m := b.resolve(t, "web-001")
	components := field(t, m, "Repository[internal]", "components")
	if len(components.List) != 1 || components.List[0].Scalar != "main" {
		t.Errorf("components = %v, want just main", components.List)
	}
}

func TestRequiresCombinesAsASet(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("base", 0, document.Matcher{}).
		layer("roles/web", 30, matchRole("web"))

	pkg := document.Reference{Type: "Package", Name: "nginx"}
	dir := document.Reference{Type: "Directory", Name: "conf"}

	b.set.Resources = append(b.set.Resources,
		document.Resource{
			Type: "File", Name: "nginx-config", Layer: "base", LayerDir: "base",
			Requires: []document.Reference{pkg},
			Desired:  document.Value{Kind: document.KindMap, Map: map[string]document.Value{"path": document.Scalar("/etc/nginx/nginx.conf")}},
		},
		document.Resource{
			Type: "File", Name: "nginx-config", Layer: "roles/web", LayerDir: "roles/web",
			Requires: []document.Reference{pkg, dir},
			Desired:  document.Value{Kind: document.KindMap, Map: map[string]document.Value{}},
		},
	)

	m := b.resolve(t, "web-001")
	var requires []string
	for _, r := range m.Resources[0].Requires {
		requires = append(requires, r.String())
	}
	want := "Package[nginx],Directory[conf]"
	if got := strings.Join(requires, ","); got != want {
		t.Errorf("requires = %s, want %s", got, want)
	}
}

func TestHostLabelIsInjected(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("hosts/web-001", 100, document.Matcher{Labels: map[string]string{document.HostLabel: "web-001"}}).
		resource("hosts/web-001", "Package", "curl", map[string]string{"state": "present"})

	m := b.resolve(t, "web-001")
	if len(m.Resources) != 1 {
		t.Fatalf("the host override layer should have matched, got %d resources", len(m.Resources))
	}
	if len(m.Layers) != 1 || m.Layers[0].Reasons.String() != "datum/host=web-001" {
		t.Errorf("layers = %v", m.Layers)
	}
}

func TestUnknownHostIsAnError(t *testing.T) {
	b := (&builder{}).layer("base", 0, document.Matcher{})
	_, err := Host(b.set, "nope", "8b91f20")
	if err == nil || !strings.Contains(err.Error(), "no Host document named") {
		t.Fatalf("err = %v", err)
	}
}

func TestSubstitutionUsesDeclaredLabels(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("roles/web", 30, matchRole("web")).
		resource("roles/web", "File", "site", map[string]string{
			"path": "/etc/app/{{ labels.site }}.conf",
			"log":  "/var/log/{{ host }}.log",
		})

	m := b.resolve(t, "web-001")
	if got := field(t, m, "File[site]", "path").Scalar; got != "/etc/app/london.conf" {
		t.Errorf("path = %q", got)
	}
	if got := field(t, m, "File[site]", "log").Scalar; got != "/var/log/web-001.log" {
		t.Errorf("log = %q", got)
	}

	used := m.Resources[0].Used
	if len(used) != 2 || used[0].Placeholder != "host" || used[1].Placeholder != "labels.site" {
		t.Errorf("used = %v", used)
	}
}

func TestSubstitutionOfUndeclaredLabelIsAnError(t *testing.T) {
	b := (&builder{}).
		host("web-001", map[string]string{"role": "web"}).
		layer("roles/web", 30, matchRole("web")).
		resource("roles/web", "File", "site", map[string]string{"path": "/etc/app/{{ labels.site }}.conf"})

	_, err := Host(b.set, "web-001", "8b91f20")
	if err == nil || !strings.Contains(err.Error(), "labels.site") {
		t.Fatalf("err = %v, want a complaint about labels.site", err)
	}
}

// Secret placeholders survive resolution untouched. They are resolved on the host
// during apply, which is what keeps a secret out of the manifest and its digest.
func TestSecretPlaceholdersAreLeftAlone(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("roles/web", 30, matchRole("web")).
		resource("roles/web", "File", "app", map[string]string{"content": "password={{ secrets.db }}"})

	m := b.resolve(t, "web-001")
	if got := field(t, m, "File[app]", "content").Scalar; got != "password={{ secrets.db }}" {
		t.Errorf("content = %q, want the placeholder kept", got)
	}
	if len(m.Resources[0].Used) != 0 {
		t.Errorf("a secret should not be reported as a used label: %v", m.Resources[0].Used)
	}
}

func TestUnknownPlaceholderIsAnError(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("roles/web", 30, matchRole("web")).
		resource("roles/web", "File", "app", map[string]string{"path": "/etc/{{ facts.os }}"})

	_, err := Host(b.set, "web-001", "8b91f20")
	if err == nil || !strings.Contains(err.Error(), "unknown substitution") {
		t.Fatalf("err = %v", err)
	}
}

// A label value containing braces is used literally. Rendering it again would let
// a repository build a reference out of label values.
func TestSubstitutedValuesAreNotRenderedAgain(t *testing.T) {
	b := (&builder{}).
		host("web-001", map[string]string{"role": "web", "site": "{{ host }}"}).
		layer("roles/web", 30, matchRole("web")).
		resource("roles/web", "File", "app", map[string]string{"path": "/etc/{{ labels.site }}"})

	m := b.resolve(t, "web-001")
	if got := field(t, m, "File[app]", "path").Scalar; got != "/etc/{{ host }}" {
		t.Errorf("path = %q, want the label value used literally", got)
	}
}

func TestDigestIsStableAndSensitiveToContent(t *testing.T) {
	build := func(mode string) Manifest {
		b := (&builder{}).
			host("web-001", labelsOf("web")).
			layer("roles/web", 30, matchRole("web")).
			resource("roles/web", "File", "nginx-config", map[string]string{"mode": mode})
		return b.resolve(t, "web-001")
	}

	first, second := build("0640"), build("0640")
	if first.Digest() != second.Digest() {
		t.Error("the same desired state produced different digests")
	}
	if first.Digest() == build("0600").Digest() {
		t.Error("different desired state produced the same digest")
	}
}

// Moving a resource between layers changes its provenance and not what happens on
// the host, so the digest must not move.
func TestDigestIgnoresProvenance(t *testing.T) {
	inRoleLayer := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("roles/web", 30, matchRole("web")).
		resource("roles/web", "Package", "nginx", map[string]string{"state": "present"}).
		resolve(t, "web-001")

	inBaseLayer := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("base", 0, document.Matcher{}).
		resource("base", "Package", "nginx", map[string]string{"state": "present"}).
		resolve(t, "web-001")

	if inRoleLayer.Digest() != inBaseLayer.Digest() {
		t.Error("moving a resource between layers changed the digest")
	}
}

func TestDigestIgnoresDependencyOrder(t *testing.T) {
	a := document.Reference{Type: "Package", Name: "a"}
	c := document.Reference{Type: "Package", Name: "c"}

	build := func(refs []document.Reference) Manifest {
		b := (&builder{}).host("web-001", labelsOf("web")).layer("base", 0, document.Matcher{})
		b.set.Resources = append(b.set.Resources, document.Resource{
			Type: "File", Name: "f", Layer: "base", LayerDir: "base",
			Requires: refs,
			Desired:  document.Value{Kind: document.KindMap, Map: map[string]document.Value{}},
		})
		return b.resolve(t, "web-001")
	}

	if build([]document.Reference{a, c}).Digest() != build([]document.Reference{c, a}).Digest() {
		t.Error("the order requires were written in changed the digest")
	}
}

func TestShortDigest(t *testing.T) {
	m := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("base", 0, document.Matcher{}).
		resource("base", "Package", "curl", map[string]string{"state": "present"}).
		resolve(t, "web-001")

	short := m.ShortDigest()
	if !strings.HasPrefix(short, "sha256:") || len(short) != len("sha256:")+8 {
		t.Errorf("ShortDigest = %q", short)
	}
	if !strings.HasPrefix(m.Digest(), short) {
		t.Errorf("%q is not a prefix of %q", short, m.Digest())
	}
}

func TestRestartOnAndReloadOnCannotMergeTogether(t *testing.T) {
	b := (&builder{}).
		host("web-001", labelsOf("web")).
		layer("base", 0, document.Matcher{}).
		layer("roles/web", 30, matchRole("web"))

	cfg := document.Reference{Type: "File", Name: "nginx-config"}
	b.set.Resources = append(b.set.Resources,
		document.Resource{
			Type: "Service", Name: "nginx", Layer: "base", LayerDir: "base",
			RestartOn: []document.Reference{cfg},
			Desired:   document.Value{Kind: document.KindMap, Map: map[string]document.Value{}},
		},
		document.Resource{
			Type: "Service", Name: "nginx", Layer: "roles/web", LayerDir: "roles/web",
			ReloadOn: []document.Reference{cfg},
			Desired:  document.Value{Kind: document.KindMap, Map: map[string]document.Value{}},
		},
	)

	_, err := Host(b.set, "web-001", "8b91f20")
	if err == nil || !strings.Contains(err.Error(), "both restartOn and reloadOn") {
		t.Fatalf("err = %v", err)
	}
}
