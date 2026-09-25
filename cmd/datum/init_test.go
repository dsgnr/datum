// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesARepositoryThatValidates(t *testing.T) {
	dir := t.TempDir()
	got := invoke("init", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	for _, want := range []string{
		"created fleet/datum.yaml",
		"created fleet/base/layer.yaml",
		"next steps",
		"add a Host document under fleet/hosts/",
	} {
		if !strings.Contains(got.all(), want) {
			t.Errorf("output should contain %q, got:\n%s", want, got.all())
		}
	}

	// The generated repository has no hosts, so validate resolves none and still
	// succeeds. Anything else means init wrote something Datum cannot read.
	after := invoke("validate", "-repo", dir)
	if after.code != exitOK {
		t.Fatalf("validate on a generated repository failed:\n%s", after.all())
	}
	if !strings.Contains(after.out, "resolved 0 hosts, 0 errors") {
		t.Errorf("validate output:\n%s", after.out)
	}
}

func TestInitWithExamplesValidates(t *testing.T) {
	dir := t.TempDir()
	got := invoke("init", "-repo", dir, "-with-examples")
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.all(), "created fleet/hosts/example-host.yaml") {
		t.Errorf("output should list the example host, got:\n%s", got.all())
	}

	after := invoke("validate", "-repo", dir)
	if after.code != exitOK {
		t.Fatalf("validate on a generated repository failed:\n%s", after.all())
	}
	if !strings.Contains(after.out, "resolved 1 hosts, 0 errors") {
		t.Errorf("validate output:\n%s", after.out)
	}
}

func TestInitNamesTheFleet(t *testing.T) {
	dir := t.TempDir()
	if got := invoke("init", "-repo", dir, "-name", "platform"); got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	after := invoke("validate", "-repo", dir)
	if !strings.Contains(after.out, "fleet      platform") {
		t.Errorf("validate output:\n%s", after.out)
	}
}

func TestInitRejectsABadFleetName(t *testing.T) {
	dir := t.TempDir()
	got := invoke("init", "-repo", dir, "-name", "not a name")
	if got.code != exitError {
		t.Fatalf("code = %d, want a failure", got.code)
	}
	if !strings.Contains(got.err, "is not allowed") {
		t.Errorf("stderr = %q", got.err)
	}
	if _, err := os.Stat(filepath.Join(dir, "fleet")); !os.IsNotExist(err) {
		t.Errorf("nothing should have been written: %v", err)
	}
}

func TestInitDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	if got := invoke("init", "-repo", dir); got.code != exitOK {
		t.Fatalf("first run failed:\n%s", got.all())
	}
	mine := filepath.Join(dir, "fleet", "datum.yaml")
	before, err := os.ReadFile(mine)
	if err != nil {
		t.Fatal(err)
	}

	again := invoke("init", "-repo", dir, "-name", "other")
	if again.code != exitError {
		t.Fatalf("a second run should fail, got code %d:\n%s", again.code, again.all())
	}
	if !strings.Contains(again.err, "already exists") {
		t.Errorf("stderr = %q", again.err)
	}
	after, err := os.ReadFile(mine)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("the existing document was rewritten:\n%s", after)
	}
}

// A directory outside a work tree still gets a repository, because adding Datum to
// something that is not yet a git repository is legitimate. It gets told, because
// desired state has no effect until it is committed.
func TestInitWarnsOutsideAWorkTree(t *testing.T) {
	dir := t.TempDir()
	got := invoke("init", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if !strings.Contains(got.err, "not inside a git work tree") {
		t.Errorf("stderr should carry the warning, got %q", got.err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Error("init should not have created a git repository")
	}
}

func TestInitIsQuietInsideAWorkTree(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	cmd := exec.Command(git, "init", "--quiet", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git init failed: %v %s", err, out)
	}

	got := invoke("init", "-repo", dir)
	if got.code != exitOK {
		t.Fatalf("code = %d, output:\n%s", got.code, got.all())
	}
	if strings.Contains(got.err, "not inside a git work tree") {
		t.Errorf("there should be no warning inside a work tree, got %q", got.err)
	}
}
