// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitFleet writes a repository, commits it, applies changes, and commits again.
// It returns the directory, so a test can compare HEAD against HEAD~1.
func gitFleet(t *testing.T, first map[string]string, second map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := fleet(t, first)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		// Signing and identity come from the environment otherwise, and a machine
		// without them configured would fail for an unrelated reason.
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("init", "-q")
	run("add", "-A")
	run("-c", "commit.gpgsign=false", "commit", "-q", "-m", "initial")

	for name, body := range second {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "-A")
	// --allow-empty so a test can create a second revision that changes nothing.
	run("-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "change")
	return dir
}

func TestAffectedReportsOnlyTheHostsAChangeReaches(t *testing.T) {
	first := worked()
	// A second host that the web role does not match.
	first["fleet/hosts/db-001/host.yaml"] = `datum: v1alpha1
type: Host
name: db-001
labels:
  environment: production
  site: london
  role: database
  architecture: amd64
`
	// The change touches the web role only.
	second := map[string]string{
		"fleet/roles/web/tuning.yaml": `datum: v1alpha1
type: File
name: nginx-tuning
desired:
  path: /etc/nginx/conf.d/tuning.conf
  owner: root
  group: root
  mode: "0644"
  content: "worker_connections 4096;"
`,
	}

	dir := gitFleet(t, first, second)
	got := invoke("affected", "-from", "HEAD~1", "-to", "HEAD", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}

	if !strings.Contains(got.out, "1 of 2 hosts affected") {
		t.Errorf("want 1 of 2 affected, got:\n%s", got.out)
	}
	if !strings.Contains(got.out, "db-001") || !strings.Contains(got.out, "unchanged") {
		t.Errorf("db-001 should be unchanged, got:\n%s", got.out)
	}
	if !strings.Contains(got.out, "web-001") || !strings.Contains(got.out, "->") {
		t.Errorf("web-001 should have a changed digest, got:\n%s", got.out)
	}
}

func TestAffectedShowsWhichResourcesChanged(t *testing.T) {
	second := map[string]string{
		"fleet/hosts/web-001/overrides.yaml": `datum: v1alpha1
type: File
name: nginx-config
desired:
  mode: "0400"
`,
	}
	dir := gitFleet(t, worked(), second)

	got := invoke("affected", "-from", "HEAD~1", "-to", "HEAD", "-repo", dir,
		"-show-resources", "-host", "web-001")
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	for _, want := range []string{"File[nginx-config]", "mode", "0600 -> 0400"} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.out)
		}
	}
}

// A change that alters no resolved desired state affects no host, whatever changed in
// the repository.
func TestAffectedReportsNothingForACommentOnlyChange(t *testing.T) {
	second := map[string]string{
		"fleet/base/layer.yaml": `# Applies to every host in the fleet.
datum: v1alpha1
type: Layer
name: base
precedence: 0
`,
	}
	dir := gitFleet(t, worked(), second)

	got := invoke("affected", "-from", "HEAD~1", "-to", "HEAD", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.out, "0 of 1 hosts affected") {
		t.Errorf("want nothing affected, got:\n%s", got.out)
	}
}

func TestAffectedReportsAnAddedHost(t *testing.T) {
	second := map[string]string{
		"fleet/hosts/web-002/host.yaml": `datum: v1alpha1
type: Host
name: web-002
labels:
  environment: production
  site: london
  role: web
  architecture: amd64
`,
	}
	dir := gitFleet(t, worked(), second)

	got := invoke("affected", "-from", "HEAD~1", "-to", "HEAD", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.out, "web-002") || !strings.Contains(got.out, "added") {
		t.Errorf("web-002 should be reported as added, got:\n%s", got.out)
	}
}

func TestAffectedNeedsFrom(t *testing.T) {
	dir := fleet(t, worked())
	got := invoke("affected", "-repo", dir)
	if got.code != exitError || !strings.Contains(got.err, "--from is required") {
		t.Errorf("code = %d stderr = %q", got.code, got.err)
	}
}

func TestAffectedShowResourcesNeedsAHost(t *testing.T) {
	dir := gitFleet(t, worked(), map[string]string{})
	got := invoke("affected", "-from", "HEAD", "-to", "HEAD", "-repo", dir, "-show-resources")
	if got.code != exitError || !strings.Contains(got.err, "needs --host") {
		t.Errorf("code = %d stderr = %q", got.code, got.err)
	}
}

func TestAffectedRejectsAnUnknownRevision(t *testing.T) {
	dir := gitFleet(t, worked(), map[string]string{})
	got := invoke("affected", "-from", "nosuchrevision", "-to", "HEAD", "-repo", dir)
	if got.code != exitError || !strings.Contains(got.err, "cannot read revision") {
		t.Errorf("code = %d stderr = %q", got.code, got.err)
	}
}

// nested returns file names moved under a subdirectory, which is how a fleet sits in a
// repository that holds other things as well.
func nested(files map[string]string, under string) map[string]string {
	out := make(map[string]string, len(files)+1)
	for name, body := range files {
		out[filepath.Join(under, name)] = body
	}
	return out
}

// The fleet is not always the repository root, and a repository can hold more than one.
// Reading history has to look in the same subdirectory the command was pointed at,
// because a worktree is a checkout of the whole repository and walking its root finds
// every fleet in it.
func TestAffectedWorksWhenTheFleetIsASubdirectory(t *testing.T) {
	first := nested(worked(), "infra/production")
	// A second fleet elsewhere in the same repository, which is what makes walking
	// the worktree root ambiguous.
	for name, body := range nested(worked(), "infra/staging") {
		first[name] = body
	}
	first["services/api/main.go"] = "package main\n"

	second := nested(map[string]string{
		"fleet/roles/web/files/nginx.conf": "worker_processes 4;\n",
	}, "infra/production")

	repo := gitFleet(t, first, second)
	fleetDir := filepath.Join(repo, "infra", "production", "fleet")

	got := invoke("affected", "-from", "HEAD~1", "-to", "HEAD", "-repo", fleetDir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.out, "web-001") {
		t.Errorf("output should name the affected host, got:\n%s", got.out)
	}
	if strings.Contains(got.all(), "more than one Fleet") {
		t.Errorf("only the fleet that was asked for should have been read, got:\n%s", got.all())
	}
}

// The other commands take the fleet directory directly, so they only need the revision
// to come from the repository the subdirectory belongs to.
func TestRevisionIsFoundFromASubdirectory(t *testing.T) {
	repo := gitFleet(t, nested(worked(), "infra"), nil)
	fleetDir := filepath.Join(repo, "infra", "fleet")

	where := inspect(fleetDir)
	if where.Toplevel == "" {
		t.Fatal("the repository root should have been found")
	}
	if where.Prefix != "infra/fleet" {
		t.Errorf("Prefix = %q, want infra/fleet", where.Prefix)
	}
	if got := where.Revision(); got == "(no revision)" {
		t.Error("the revision should have been read")
	}

	got := invoke("render", "-host", "web-001", "-repo", fleetDir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
}

// A fleet at the repository root has no prefix, and nothing should change for it.
func TestFleetAtTheRepositoryRootHasNoPrefix(t *testing.T) {
	repo := gitFleet(t, worked(), nil)

	where := inspect(repo)
	if where.Prefix != "" {
		t.Errorf("Prefix = %q, want empty", where.Prefix)
	}
	if where.fleetIn("/tmp/tree") != "/tmp/tree" {
		t.Errorf("fleetIn = %q", where.fleetIn("/tmp/tree"))
	}
}

// Outside a work tree there is no revision and no root, and the read-only commands
// still work because resolution does not need either.
func TestDirectoryOutsideAGitRepository(t *testing.T) {
	dir := fleet(t, worked())

	where := inspect(dir)
	if where.Toplevel != "" {
		t.Errorf("Toplevel = %q, want empty", where.Toplevel)
	}
	if got := where.Revision(); got != "(no revision)" {
		t.Errorf("Revision = %q", got)
	}

	if got := invoke("render", "-host", "web-001", "-repo", dir); got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
}

func TestAffectedOutsideAGitRepositoryIsAnError(t *testing.T) {
	dir := fleet(t, worked())

	got := invoke("affected", "-from", "HEAD~1", "-repo", dir)
	if got.code != exitError {
		t.Fatalf("code = %d, want %d", got.code, exitError)
	}
	if !strings.Contains(got.err, "not inside a git repository") {
		t.Errorf("stderr = %q", got.err)
	}
}
