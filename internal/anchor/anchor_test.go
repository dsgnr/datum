// SPDX-License-Identifier: Apache-2.0

package anchor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/anchor"
	"github.com/dsgnr/datum/internal/config"
	"github.com/dsgnr/datum/internal/document"
)

func ref(typeName, name string) document.Reference {
	return document.Reference{Type: typeName, Name: name}
}

func desired(pairs ...string) document.Value {
	value := document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}
	for i := 0; i+1 < len(pairs); i += 2 {
		value.Map[pairs[i]] = document.Scalar(pairs[i+1])
	}
	return value
}

// stock is a host running with the documented paths.
func stock() anchor.Set { return anchor.Default() }

// Every path in the documentation's table has to be refused, or the table is decoration.
func TestEveryDocumentedPathIsRefused(t *testing.T) {
	set := stock()
	for _, target := range []string{
		"/etc/datum/agent.yaml",
		"/etc/datum/allowed-signers",
		"/etc/datum/credentials/git",
		"/etc/datum/secrets/app-password",
		"/var/lib/datum/accepted-revision",
		"/var/lib/datum/reports/20260208T091422.000Z.json",
		"/var/lib/datum",
		"/usr/bin/datum",
	} {
		if err := set.Check(ref("File", "x"), target, desired("path", target)); err == nil {
			t.Errorf("%s was allowed", target)
		}
	}
}

// The error has to name the control, or a refusal reads as a manifest error.
func TestTheRefusalNamesWhatItProtects(t *testing.T) {
	err := stock().Check(ref("File", "signers"), "/etc/datum/allowed-signers",
		desired("path", "/etc/datum/allowed-signers"))
	if err == nil {
		t.Fatal("allowed as it is")
	}
	for _, want := range []string{"File[signers]", "/etc/datum/allowed-signers", "authorise desired state"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// Each of these is a route the documentation says a string comparison would miss.
func TestTheRoutesAStringComparisonWouldMiss(t *testing.T) {
	set := stock()

	cases := []struct {
		name    string
		ref     document.Reference
		target  string
		desired document.Value
	}{
		{
			name:   "a path that cleans onto a protected one",
			ref:    ref("File", "sneaky"),
			target: "/etc/datum/../datum/agent.yaml",
		},
		{
			name:   "a deeper traversal onto a protected one",
			ref:    ref("File", "deeper"),
			target: "/etc/foo/../datum/allowed-signers",
		},
		{
			name:   "a directory that is an ancestor of protected state",
			ref:    ref("Directory", "etc"),
			target: "/etc",
		},
		{
			name:   "a directory that is protected state",
			ref:    ref("Directory", "datum"),
			target: "/etc/datum",
		},
		{
			name:   "a file beneath a protected directory",
			ref:    ref("File", "secret"),
			target: "/etc/datum/secrets/anything",
		},
		{
			name:   "a directory above the state directory",
			ref:    ref("Directory", "lib"),
			target: "/var/lib",
		},
		{
			name:   "a symlink at a protected path, whatever it points at",
			ref:    ref("Symlink", "link"),
			target: "/etc/datum/allowed-signers",
			desired: desired("path", "/etc/datum/allowed-signers",
				"target", "/tmp/innocent"),
		},
		{
			name:   "a symlink elsewhere pointing into protected state",
			ref:    ref("Symlink", "pointer"),
			target: "/tmp/innocent",
			desired: desired("path", "/tmp/innocent",
				"target", "/etc/datum/allowed-signers"),
		},
	}

	for _, tc := range cases {
		d := tc.desired
		if d.Kind == 0 {
			d = desired("path", tc.target)
		}
		if err := set.Check(tc.ref, tc.target, d); err == nil {
			t.Errorf("%s was allowed (%s)", tc.name, tc.target)
		}
	}
}

// Datum exists to manage system configuration, so the refusal has to be narrow enough to
// leave that possible.
func TestOrdinaryConfigurationIsStillAllowed(t *testing.T) {
	set := stock()
	for _, target := range []string{
		"/etc/nginx/nginx.conf",
		"/etc/sudoers.d/ops",
		"/etc/systemd/system/app.service",
		"/root/.ssh/authorized_keys",
		"/etc/datumish/config",
		"/etc/datum-other/thing",
		"/var/lib/postgresql",
		"/var/lib/datumish",
		"/usr/bin/datumctl",
		"/usr/local/bin/datum",
	} {
		if err := set.Check(ref("File", "x"), target, desired("path", target)); err != nil {
			t.Errorf("%s was refused: %v", target, err)
		}
	}
}

// A resource that removes the agent's package or stops its service reaches the same
// outcome without naming any protected path.
func TestTheAgentsOwnPackageAndServiceAreRefused(t *testing.T) {
	set := stock()
	if err := set.Check(ref("Package", "datum"), "datum", desired("state", "absent")); err == nil {
		t.Error("Package[datum] was allowed")
	}
	if err := set.Check(ref("Service", "datum"), "datum.service", desired("state", "stopped")); err == nil {
		t.Error("Service[datum.service] was allowed")
	}
	// The unit is often written without its suffix.
	if err := set.Check(ref("Service", "datum"), "datum", desired("state", "stopped")); err == nil {
		t.Error("Service[datum] was allowed")
	}
	// Anything else of those types is ordinary.
	if err := set.Check(ref("Package", "nginx"), "nginx", desired("state", "present")); err != nil {
		t.Errorf("Package[nginx] was refused: %v", err)
	}
	if err := set.Check(ref("Service", "nginx"), "nginx.service", desired("state", "running")); err != nil {
		t.Errorf("Service[nginx] was refused: %v", err)
	}
}

// Root is deliberately not protected. Datum manages users and root is a user, so
// protecting it would extend the list to most of /etc.
func TestRootIsNotProtected(t *testing.T) {
	if err := stock().Check(ref("User", "root"), "root", desired("shell", "/bin/false")); err != nil {
		t.Errorf("User[root] was refused: %v", err)
	}
}

// A host that keeps its state elsewhere needs that directory protected, so the
// configured path is used rather than the default.
func TestTheSetFollowsTheConfiguration(t *testing.T) {
	cfg := config.Default()
	cfg.Host = "web-001"
	cfg.State = "/srv/datum-state"
	cfg.Trust.Signers = "/opt/keys/signers"
	cfg.Source.Credential = "/opt/keys/deploy"
	cfg.Secrets.Path = "/opt/secrets"
	set := anchor.For(cfg)

	for _, target := range []string{
		"/srv/datum-state/accepted-revision",
		"/srv/datum-state",
		"/opt/keys/signers",
		"/opt/keys/deploy",
		"/opt/secrets/token",
	} {
		if err := set.Check(ref("File", "x"), target, desired("path", target)); err == nil {
			t.Errorf("%s was allowed on a host configured to use it", target)
		}
	}
	// And the default state directory is no longer special on that host.
	if err := set.Check(ref("File", "x"), "/var/lib/datum/other", desired("path", "/var/lib/datum/other")); err != nil {
		t.Errorf("the unused default was refused: %v", err)
	}
}

// A relative path is refused by field validation, and reporting it here as well would give
// two errors for one mistake.
func TestARelativePathIsLeftToFieldValidation(t *testing.T) {
	if err := stock().Check(ref("File", "x"), "etc/datum/agent.yaml", desired("path", "etc/datum/agent.yaml")); err != nil {
		t.Errorf("a relative path was refused here: %v", err)
	}
}

// A hard link shares an inode with the file it links to, so writing one writes the other
// and the two names have nothing in common.
func TestAHardLinkIntoProtectedStateIsRefused(t *testing.T) {
	dir := t.TempDir()
	protected := filepath.Join(dir, "allowed-signers")
	if err := os.WriteFile(protected, []byte("key\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	innocent := filepath.Join(dir, "innocent")
	if err := os.Link(protected, innocent); err != nil {
		t.Skipf("hard links are not available here: %v", err)
	}

	set := anchor.Set{Files: []string{protected}}
	// By name it looks fine, which is the point.
	if err := set.Check(ref("File", "x"), innocent, desired("path", innocent)); err != nil {
		t.Fatalf("the declared-path check should not have caught this: %v", err)
	}
	if err := set.CheckIdentity(ref("File", "x"), innocent); err == nil {
		t.Error("a hard link into protected state was allowed")
	}
}

// A link somebody already placed on the host reaches protected state through a path that
// declares nothing suspicious.
func TestAPathResolvingThroughAnExistingLinkIsRefused(t *testing.T) {
	dir := t.TempDir()
	protectedDir := filepath.Join(dir, "datum")
	if err := os.MkdirAll(protectedDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(protectedDir, "agent.yaml"), []byte("host: x\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(dir, "config")
	if err := os.Symlink(protectedDir, link); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}

	set := anchor.Set{Dirs: []string{protectedDir}}
	through := filepath.Join(link, "agent.yaml")
	if err := set.CheckIdentity(ref("File", "x"), through); err == nil {
		t.Error("a path resolving through a link into protected state was allowed")
	}
}

// Writing a Symlink replaces the link rather than following it, so the identity check must
// not resolve a Symlink's own path and refuse something legitimate.
func TestASymlinkResourceIsNotJudgedByWhatItCurrentlyPointsAt(t *testing.T) {
	dir := t.TempDir()
	protected := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(protected, []byte("host: x\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(dir, "somelink")
	if err := os.Symlink(protected, link); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}

	set := anchor.Set{Files: []string{protected}}
	// Replacing that link with one pointing somewhere harmless is allowed, because the
	// write lands on the link and not on the protected file. The declared-target check
	// is what refuses a link that points into protected state.
	if err := set.CheckIdentity(ref("Symlink", "x"), link); err != nil {
		t.Errorf("replacing a link was refused: %v", err)
	}
}

func TestAPathThatDoesNotExistIsNotAnIdentityProblem(t *testing.T) {
	set := anchor.Set{Files: []string{"/etc/datum/agent.yaml"}}
	if err := set.CheckIdentity(ref("File", "x"), filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Errorf("an absent path was refused: %v", err)
	}
}
