// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// result is what one command invocation produced.
type result struct {
	code int
	out  string
	err  string
}

func invoke(args ...string) result {
	var out, errOut bytes.Buffer
	code := run(&env{out: &out, err: &errOut}, args)
	return result{code: code, out: out.String(), err: errOut.String()}
}

func (r result) all() string { return r.out + r.err }

// fleet writes a repository and returns its directory.
func fleet(t *testing.T, files map[string]string) string {
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

// worked is the example the documentation uses throughout, trimmed to what these
// tests need.
func worked() map[string]string {
	return map[string]string{
		"fleet/datum.yaml": "datum: v1alpha1\ntype: Fleet\nname: example\n",
		"fleet/base/layer.yaml": `datum: v1alpha1
type: Layer
name: base
precedence: 0
`,
		"fleet/base/packages.yaml": `datum: v1alpha1
type: Package
name: curl
desired:
  state: present
---
datum: v1alpha1
type: Sysctl
name: net.ipv4.ip_forward
desired:
  value: "0"
`,
		"fleet/environments/production/layer.yaml": `datum: v1alpha1
type: Layer
name: environment-production
precedence: 10
match:
  labels:
    environment: production
`,
		"fleet/environments/production/sysctl.yaml": `datum: v1alpha1
type: Sysctl
name: net.ipv4.ip_forward
desired:
  value: "1"
`,
		"fleet/roles/web/layer.yaml": `datum: v1alpha1
type: Layer
name: role-web
precedence: 30
match:
  labels:
    role: web
`,
		"fleet/roles/web/nginx.yaml": `datum: v1alpha1
type: Package
name: nginx
desired:
  state: present
---
datum: v1alpha1
type: File
name: nginx-config
requires:
  - Package[nginx]
desired:
  path: /etc/nginx/nginx.conf
  owner: root
  group: root
  mode: "0640"
  source: files/nginx.conf
---
datum: v1alpha1
type: Service
name: nginx
requires:
  - Package[nginx]
restartOn:
  - File[nginx-config]
desired:
  state: running
  enabled: true
`,
		"fleet/roles/web/files/nginx.conf": "worker_processes auto;\n",
		"fleet/hosts/web-001/layer.yaml": `datum: v1alpha1
type: Layer
name: host-web-001
precedence: 100
match:
  labels:
    datum/host: web-001
`,
		"fleet/hosts/web-001/host.yaml": `datum: v1alpha1
type: Host
name: web-001
labels:
  environment: production
  site: london
  role: web
  architecture: amd64
`,
		"fleet/hosts/web-001/overrides.yaml": `datum: v1alpha1
type: File
name: nginx-config
desired:
  mode: "0600"
`,
	}
}

func TestUsageListsCommands(t *testing.T) {
	got := invoke()
	if got.code != exitOK {
		t.Errorf("code = %d", got.code)
	}
	for _, want := range []string{"render", "explain", "validate"} {
		if !strings.Contains(got.out, want) {
			t.Errorf("usage should list %q, got:\n%s", want, got.out)
		}
	}
}

func TestUnknownCommandFails(t *testing.T) {
	got := invoke("reticulate")
	if got.code != exitError {
		t.Errorf("code = %d, want %d", got.code, exitError)
	}
	if !strings.Contains(got.err, "unknown command") {
		t.Errorf("stderr = %q", got.err)
	}
}

func TestValidateAcceptsAGoodFleet(t *testing.T) {
	dir := fleet(t, worked())
	got := invoke("validate", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	for _, want := range []string{"fleet      example", "hosts      1", "resolved 1 hosts, 0 errors"} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.out)
		}
	}
}

func TestValidateReportsAConflictOnce(t *testing.T) {
	files := worked()
	// A second layer at the same precedence disagreeing about one field.
	files["fleet/roles/web-tls/layer.yaml"] = `datum: v1alpha1
type: Layer
name: role-web-tls
precedence: 30
match:
  labels:
    role: web
`
	files["fleet/roles/web-tls/nginx.yaml"] = `datum: v1alpha1
type: File
name: nginx-config
desired:
  mode: "0644"
`
	dir := fleet(t, files)
	got := invoke("validate", "-repo", dir)

	if got.code != exitError {
		t.Fatalf("code = %d, want a failure. output:\n%s", got.code, got.all())
	}
	for _, want := range []string{"conflicting values", "File[nginx-config].mode", "affects 1 host", "first: web-001"} {
		if !strings.Contains(got.all(), want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.all())
		}
	}
	if n := strings.Count(got.all(), "conflicting values"); n != 1 {
		t.Errorf("the conflict was reported %d times, want once", n)
	}
}

func TestValidateReportsADependencyCycle(t *testing.T) {
	files := worked()
	files["fleet/roles/web/cycle.yaml"] = `datum: v1alpha1
type: Package
name: looped
requires:
  - Service[nginx]
desired:
  state: present
---
datum: v1alpha1
type: Group
name: looper
requires:
  - Package[looped]
desired:
  state: present
`
	// Make the service depend on the group, closing the loop.
	files["fleet/roles/web/nginx.yaml"] = strings.Replace(
		files["fleet/roles/web/nginx.yaml"],
		"requires:\n  - Package[nginx]\nrestartOn:",
		"requires:\n  - Package[nginx]\n  - Group[looper]\nrestartOn:", 1)

	dir := fleet(t, files)
	got := invoke("validate", "-repo", dir)
	if got.code != exitError {
		t.Fatalf("code = %d, want a failure. output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.all(), "dependency cycle") {
		t.Errorf("output should report a cycle, got:\n%s", got.all())
	}
}

func TestValidateNeedsAFleetDocument(t *testing.T) {
	dir := fleet(t, map[string]string{
		"fleet/base/layer.yaml": "datum: v1alpha1\ntype: Layer\nname: base\n",
	})
	got := invoke("validate", "-repo", dir)
	if got.code != exitError {
		t.Errorf("code = %d", got.code)
	}
	if !strings.Contains(got.err, "no Fleet document") {
		t.Errorf("stderr = %q", got.err)
	}
}

func TestRenderPrintsTheManifest(t *testing.T) {
	dir := fleet(t, worked())
	got := invoke("render", "-host", "web-001", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	for _, want := range []string{
		"host       web-001",
		"resources  5",
		"File[nginx-config]",
		"role-web, host-web-001",
		"Sysctl[net.ipv4.ip_forward]",
	} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.out)
		}
	}
}

// The same repository and host has to produce the same digest every time, because
// everything built on content addressing depends on it.
func TestRenderDigestIsStable(t *testing.T) {
	dir := fleet(t, worked())
	first := invoke("render", "-host", "web-001", "-repo", dir)
	for i := 0; i < 5; i++ {
		again := invoke("render", "-host", "web-001", "-repo", dir)
		if again.out != first.out {
			t.Fatalf("render output changed between runs:\n%s\n%s", first.out, again.out)
		}
	}
}

func TestRenderNeedsAHost(t *testing.T) {
	dir := fleet(t, worked())
	got := invoke("render", "-repo", dir)
	if got.code != exitError || !strings.Contains(got.err, "--host is required") {
		t.Errorf("code = %d stderr = %q", got.code, got.err)
	}
}

func TestRenderRejectsAnUnknownHost(t *testing.T) {
	dir := fleet(t, worked())
	got := invoke("render", "-host", "nope", "-repo", dir)
	if got.code != exitError || !strings.Contains(got.err, "no Host document named") {
		t.Errorf("code = %d stderr = %q", got.code, got.err)
	}
}

func TestExplainShowsProvenance(t *testing.T) {
	dir := fleet(t, worked())
	got := invoke("explain", "File[nginx-config]", "-host", "web-001", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	for _, want := range []string{
		"File[nginx-config]   /etc/nginx/nginx.conf",
		"contributed by",
		"role-web",
		"precedence 30",
		"matched role=web",
		"host-web-001",
		"precedence 100",
		"matched datum/host=web-001",
		"overrides 0640 from role-web",
		"requires",
		"Package[nginx]",
	} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.out)
		}
	}
}

// Flags are allowed after the positional argument, which is how the documented
// usage writes it.
func TestExplainAcceptsFlagsAfterTheReference(t *testing.T) {
	dir := fleet(t, worked())
	got := invoke("explain", "Sysctl[net.ipv4.ip_forward]", "-host", "web-001", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.out, "overrides 0 from base") {
		t.Errorf("output should show the base value being overridden, got:\n%s", got.out)
	}
}

func TestExplainRejectsABadReference(t *testing.T) {
	dir := fleet(t, worked())
	got := invoke("explain", "nginx", "-host", "web-001", "-repo", dir)
	if got.code != exitError || !strings.Contains(got.err, "not a resource reference") {
		t.Errorf("code = %d stderr = %q", got.code, got.err)
	}
}

func TestExplainReportsAResourceThatDoesNotApply(t *testing.T) {
	dir := fleet(t, worked())
	got := invoke("explain", "Package[postgresql]", "-host", "web-001", "-repo", dir)
	if got.code != exitError || !strings.Contains(got.err, "does not apply to web-001") {
		t.Errorf("code = %d stderr = %q", got.code, got.err)
	}
}

func TestSubstitutionEndToEnd(t *testing.T) {
	files := worked()
	files["fleet/roles/web/site.yaml"] = `datum: v1alpha1
type: File
name: site-config
desired:
  path: /etc/app/{{ labels.site }}.conf
  owner: root
  group: root
  mode: "0640"
  content: "host is {{ host }}"
`
	dir := fleet(t, files)
	got := invoke("explain", "File[site-config]", "-host", "web-001", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	for _, want := range []string{
		"/etc/app/london.conf",
		"host is web-001",
		"substitutions",
		"labels.site",
		"london",
	} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.out)
		}
	}
}

func TestSubstitutionOfAnUndeclaredLabelFails(t *testing.T) {
	files := worked()
	files["fleet/roles/web/site.yaml"] = `datum: v1alpha1
type: File
name: site-config
desired:
  path: /etc/app/{{ labels.datacentre }}.conf
`
	dir := fleet(t, files)
	got := invoke("validate", "-repo", dir)
	if got.code != exitError {
		t.Fatalf("code = %d, want a failure. output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.all(), "labels.datacentre") {
		t.Errorf("output should name the missing label, got:\n%s", got.all())
	}
}

// Several mistakes on one host have to count as several, or a repository with four
// problems looks like it has one.
func TestValidateCountsEachProblemSeparately(t *testing.T) {
	files := worked()
	files["fleet/roles/web/bad.yaml"] = `datum: v1alpha1
type: File
name: helper
desired:
  path: /usr/local/bin/helper
  mode: "4755"
---
datum: v1alpha1
type: File
name: typo
desired:
  path: /etc/app.conf
  contnet: hello
`
	dir := fleet(t, files)
	got := invoke("validate", "-repo", dir)

	if got.code != exitError {
		t.Fatalf("code = %d, want a failure. output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.out, "2 errors") {
		t.Errorf("want 2 errors reported, got:\n%s", got.all())
	}
	for _, want := range []string{"allowPrivileged", "no field desired.contnet"} {
		if !strings.Contains(got.all(), want) {
			t.Errorf("output should mention %q, got:\n%s", want, got.all())
		}
	}
}
