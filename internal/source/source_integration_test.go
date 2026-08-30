//go:build integration

// SPDX-License-Identifier: Apache-2.0

// These tests drive the whole revision-selection policy against a real repository with
// real signatures, covering fetch, choosing a candidate, verifying it, checking it is
// newer, and falling back when any of that refuses.
//
// The fallback path is tested most heavily, since a fallback that fails is
// indistinguishable from one that was never exercised.
package source_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsgnr/datum/internal/config"
	"github.com/dsgnr/datum/internal/revision"
	"github.com/dsgnr/datum/internal/source"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return c
}

// fleet is an origin repository holding a minimal but real fleet, plus the signing key.
type fleet struct {
	dir     string
	signers string
	key     string
	home    string
	state   string
}

const fleetDoc = `datum: v1alpha1
type: Fleet
name: example
`

func hostDoc(name string) string {
	return "datum: v1alpha1\ntype: Host\nname: " + name + "\nlabels:\n  role: web\n"
}

func layerDoc() string {
	return "datum: v1alpha1\ntype: Layer\nname: base\nmatch:\n  labels:\n    role: web\n"
}

func sysctlDoc(value string) string {
	return "datum: v1alpha1\ntype: Sysctl\nname: vm.swappiness\ndesired:\n  value: \"" + value + "\"\n"
}

func newFleet(t *testing.T) *fleet {
	t.Helper()
	for _, tool := range []string{"git", "ssh-keygen"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}

	base := t.TempDir()
	f := &fleet{
		dir:     filepath.Join(base, "origin"),
		signers: filepath.Join(base, "allowed-signers"),
		key:     filepath.Join(base, "id_ed25519"),
		home:    filepath.Join(base, "home"),
		state:   filepath.Join(base, "state"),
	}
	for _, dir := range []string{f.home, f.state} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-C", "fleet@example.com",
		"-f", f.key).CombinedOutput()
	if err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	pub, err := os.ReadFile(f.key + ".pub")
	if err != nil {
		t.Fatalf("reading public key: %v", err)
	}
	if err := os.WriteFile(f.signers, []byte("fleet@example.com "+string(pub)), 0o644); err != nil {
		t.Fatalf("writing signers: %v", err)
	}

	f.git(t, "init", "--initial-branch=main", f.dir)
	f.inRepo(t, "config", "user.name", "Fleet")
	f.inRepo(t, "config", "user.email", "fleet@example.com")
	f.inRepo(t, "config", "gpg.format", "ssh")
	f.inRepo(t, "config", "user.signingkey", f.key)
	f.inRepo(t, "config", "gpg.ssh.allowedSignersFile", f.signers)

	f.write(t, "fleet.yaml", fleetDoc)
	f.write(t, "hosts/web-001.yaml", hostDoc("web-001"))
	f.write(t, "layers/base/layer.yaml", layerDoc())
	f.write(t, "layers/base/sysctl.yaml", sysctlDoc("10"))
	return f
}

func (f *fleet) git(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "HOME="+f.home, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *fleet) inRepo(t *testing.T, args ...string) string {
	t.Helper()
	return f.git(t, append([]string{"-C", f.dir}, args...)...)
}

func (f *fleet) write(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join(f.dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// commit stages everything and commits, signed unless told otherwise.
func (f *fleet) commit(t *testing.T, message string, signed bool) string {
	t.Helper()
	f.inRepo(t, "add", "-A")
	args := []string{"commit", "-m", message}
	if signed {
		args = append(args, "-S")
	} else {
		args = append(args, "--no-gpg-sign")
	}
	f.inRepo(t, args...)
	return f.inRepo(t, "rev-parse", "HEAD")
}

func (f *fleet) config(require config.Require) config.Config {
	cfg := config.Default()
	cfg.Host = "web-001"
	cfg.State = f.state
	// file:// and not a path, so the clone uses the pack protocol and transfers only
	// reachable objects. A local path clone hardlinks the whole object store, which would
	// keep commits a force-push made unreachable.
	cfg.Source.URL = "file://" + f.dir
	cfg.Source.Branch = "main"
	cfg.Trust.Require = require
	cfg.Trust.Signers = f.signers
	if require == config.RequireSignedTag {
		cfg.Trust.TagPattern = "release-*"
	}
	return cfg
}

func (f *fleet) source(t *testing.T, require config.Require) *source.Source {
	t.Helper()
	return source.New(f.config(require), "")
}

// swappiness reads the value the selected tree actually holds, which is how a test
// tells which revision was materialised rather than trusting the reported name.
func swappiness(t *testing.T, sel source.Selection) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sel.Fleet, "layers", "base", "sysctl.yaml"))
	if err != nil {
		t.Fatalf("reading the selected tree: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "value:"); ok {
			return strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	t.Fatalf("no value in:\n%s", data)
	return ""
}

func TestASignedCommitIsSelectedAndAccepted(t *testing.T) {
	f := newFleet(t)
	rev := f.commit(t, "initial", true)

	src := f.source(t, config.RequireSignedCommit)
	sel, err := src.Select(ctx(t))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if sel.Revision != rev {
		t.Errorf("selected %s, want %s", sel.Revision, rev)
	}
	if !sel.FirstContact {
		t.Error("a host with no recorded revision should be first contact")
	}
	if sel.FellBack {
		t.Error("a good revision fell back")
	}
	if sel.Signature.KeyID == "" {
		t.Error("no key was reported")
	}
	if got := swappiness(t, sel); got != "10" {
		t.Errorf("tree holds %q", got)
	}

	// The pointer advances only when told, so that apply failures cannot move it.
	if _, found, _ := revision.Read(f.state); found {
		t.Error("the pointer advanced before Accept was called")
	}
	if err := src.Accept(sel.Revision); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	accepted, found, err := revision.Read(f.state)
	if err != nil || !found || accepted != rev {
		t.Errorf("accepted = %q, %t, %v", accepted, found, err)
	}
}

func TestAnUnsignedTipIsRefusedUnderSignedCommit(t *testing.T) {
	f := newFleet(t)
	f.commit(t, "unsigned", false)

	src := f.source(t, config.RequireSignedCommit)
	_, err := src.Select(ctx(t))
	if err == nil {
		t.Fatal("an unsigned tip was accepted")
	}
	reason, ok := source.ReasonOf(err)
	if !ok || reason != source.Unsigned {
		t.Errorf("reason = %q, %t", reason, ok)
	}
}

// require: none is what a fleet that has not set up signing uses, and it has to work.
func TestRequireNoneAppliesTheTipWithoutVerifying(t *testing.T) {
	f := newFleet(t)
	rev := f.commit(t, "unsigned", false)

	sel, err := f.source(t, config.RequireNone).Select(ctx(t))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if sel.Revision != rev {
		t.Errorf("selected %s, want %s", sel.Revision, rev)
	}
	if sel.Signature.KeyID != "" {
		t.Error("a key was reported for an unverified revision")
	}
}

// The behaviour the documentation exists for, where a bad commit stops the fleet moving
// forward and does not stop it working.
func TestABadCommitLeavesTheHostOnTheAcceptedRevision(t *testing.T) {
	f := newFleet(t)
	good := f.commit(t, "good", true)

	src := f.source(t, config.RequireSignedCommit)
	sel, err := src.Select(ctx(t))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if err := src.Accept(sel.Revision); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	// A newer revision that is signed but unusable, which is the everyday case of a typo
	// merged to the tracked branch.
	f.write(t, "layers/base/sysctl.yaml", "datum: v1alpha1\ntype: Sysctl\nname: vm.swappiness\ndesired:\n  value: \"20\"\n  : broken\n")
	bad := f.commit(t, "broken yaml", true)

	after, err := src.Select(ctx(t))
	if err != nil {
		t.Fatalf("Select after the bad commit: %v", err)
	}
	// Selection itself succeeds, because the document is only parsed during resolution.
	// What matters is that the newer revision is the one selected and that the accepted
	// one is still recorded, so a resolution failure can fall back to it.
	if after.Revision != bad {
		t.Errorf("selected %s, want the new revision %s", after.Revision, bad)
	}
	if after.Accepted != good {
		t.Errorf("accepted = %s, want %s", after.Accepted, good)
	}
}

// A revision that is signed by a trusted key but older than what the host accepted is a
// downgrade, and a valid signature does not make it current.
func TestADowngradeIsRefusedAndTheHostKeepsWorking(t *testing.T) {
	f := newFleet(t)
	f.commit(t, "first", true)
	f.write(t, "layers/base/sysctl.yaml", sysctlDoc("60"))
	newer := f.commit(t, "second", true)

	src := f.source(t, config.RequireSignedCommit)
	sel, err := src.Select(ctx(t))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if err := src.Accept(sel.Revision); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if sel.Revision != newer {
		t.Fatalf("selected %s, want %s", sel.Revision, newer)
	}

	// The branch is moved back to an earlier signed revision, which is what a revert by
	// force-push looks like.
	f.inRepo(t, "reset", "--hard", "HEAD~1")

	after, err := src.Select(ctx(t))
	if err != nil {
		t.Fatalf("Select after the downgrade: %v", err)
	}
	if !after.FellBack {
		t.Fatal("a downgrade was applied")
	}
	if after.Revision != newer {
		t.Errorf("reconciling %s, want to stay on %s", after.Revision, newer)
	}
	reason, ok := source.ReasonOf(after.Cause)
	if !ok || reason != source.NotDescendant {
		t.Errorf("reason = %q, %t", reason, ok)
	}
	// Still reconciling, and against the newer content, not the reverted tree.
	if got := swappiness(t, after); got != "60" {
		t.Errorf("tree holds %q, want the accepted revision's 60", got)
	}
}

// An attacker who can force-push could otherwise remove the commit a host is pinned to
// and have that host accept anything.
func TestARewrittenHistoryIsRefusedRatherThanTreatedAsFirstContact(t *testing.T) {
	f := newFleet(t)
	f.commit(t, "first", true)
	f.write(t, "layers/base/sysctl.yaml", sysctlDoc("42"))
	doomed := f.commit(t, "second", true)

	src := f.source(t, config.RequireSignedCommit)
	if _, err := src.Select(ctx(t)); err != nil {
		t.Fatalf("Select: %v", err)
	}
	if err := src.Accept(doomed); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	// The accepted commit is removed from the local store as well, which is what a
	// force-push plus a fresh clone produces.
	if err := os.RemoveAll(filepath.Join(f.state, "repository")); err != nil {
		t.Fatalf("removing the clone: %v", err)
	}
	f.inRepo(t, "reset", "--hard", "HEAD~1")
	f.write(t, "layers/base/sysctl.yaml", sysctlDoc("99"))
	f.commit(t, "rewritten", true)

	after, err := src.Select(ctx(t))
	if err == nil && !after.FellBack {
		t.Fatal("a rewritten history was accepted")
	}
	cause := err
	if cause == nil {
		cause = after.Cause
	}
	reason, ok := source.ReasonOf(cause)
	if !ok || reason != source.NotDescendant {
		t.Errorf("reason = %q, %t, from %v", reason, ok, cause)
	}
	// The message has to say how to recover, since the host will keep refusing.
	if !strings.Contains(cause.Error(), "datum revision clear") {
		t.Errorf("the error does not say how to recover: %v", cause)
	}
}

// A host that cannot reach its source keeps reconciling what it holds, which is what
// makes Datum usable on machines that are not permanently connected.
func TestAnUnreachableSourceKeepsReconcilingTheAcceptedRevision(t *testing.T) {
	f := newFleet(t)
	f.commit(t, "initial", true)

	src := f.source(t, config.RequireSignedCommit)
	sel, err := src.Select(ctx(t))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if err := src.Accept(sel.Revision); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	// The remote goes away entirely.
	if err := os.RemoveAll(f.dir); err != nil {
		t.Fatalf("removing the origin: %v", err)
	}

	after, err := src.Select(ctx(t))
	if err != nil {
		t.Fatalf("Select with no remote: %v", err)
	}
	if !after.FellBack {
		t.Error("an unreachable source did not fall back")
	}
	if after.Revision != sel.Revision {
		t.Errorf("reconciling %s, want %s", after.Revision, sel.Revision)
	}
	if got := swappiness(t, after); got != "10" {
		t.Errorf("tree holds %q", got)
	}
}

// A host that never had a revision has nothing to fall back to, and that is the only
// case where a source problem leaves a host unmanaged.
func TestFirstContactWithAnUnreachableSourceFails(t *testing.T) {
	f := newFleet(t)
	f.commit(t, "initial", true)
	if err := os.RemoveAll(f.dir); err != nil {
		t.Fatalf("removing the origin: %v", err)
	}

	if _, err := f.source(t, config.RequireSignedCommit).Select(ctx(t)); err == nil {
		t.Fatal("a host with no accepted revision and no source reported success")
	}
}

func TestSignedTagSelectsTheTagRatherThanTheTip(t *testing.T) {
	f := newFleet(t)
	f.commit(t, "release content", true)
	tagged := f.inRepo(t, "rev-parse", "HEAD")
	f.inRepo(t, "tag", "-s", "-m", "release-1", "release-1")

	// A later unsigned commit on the branch, which signed-tag mode must not apply.
	f.write(t, "layers/base/sysctl.yaml", sysctlDoc("77"))
	tip := f.commit(t, "after the release", false)

	sel, err := f.source(t, config.RequireSignedTag).Select(ctx(t))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if sel.Revision == tip {
		t.Fatal("signed-tag mode applied the branch tip")
	}
	if sel.Revision != tagged {
		t.Errorf("selected %s, want the tagged %s", sel.Revision, tagged)
	}
	if sel.Tag != "release-1" {
		t.Errorf("tag = %q", sel.Tag)
	}
	if got := swappiness(t, sel); got != "10" {
		t.Errorf("tree holds %q, want the tagged revision's 10", got)
	}
}

// Two signed tags on diverged histories have no correct answer, and the host stays
// where it is instead of picking one.
func TestAnAmbiguousTagLeavesTheHostWhereItIs(t *testing.T) {
	f := newFleet(t)
	f.commit(t, "base", true)
	f.inRepo(t, "tag", "-s", "-m", "release-0", "release-0")

	src := f.source(t, config.RequireSignedTag)
	sel, err := src.Select(ctx(t))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if err := src.Accept(sel.Revision); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	base := sel.Revision

	// Separate files on each side, so the two histories diverge without conflicting and
	// the merge below succeeds, which is what makes both tags reachable.
	f.inRepo(t, "checkout", "-q", "-b", "left", base)
	f.write(t, "layers/base/left.yaml", "datum: v1alpha1\ntype: Sysctl\nname: vm.dirty_ratio\ndesired:\n  value: \"11\"\n")
	f.commit(t, "left", true)
	f.inRepo(t, "tag", "-s", "-m", "release-left", "release-left")

	f.inRepo(t, "checkout", "-q", "-b", "right", base)
	f.write(t, "layers/base/right.yaml", "datum: v1alpha1\ntype: Sysctl\nname: vm.dirty_background_ratio\ndesired:\n  value: \"22\"\n")
	f.commit(t, "right", true)
	f.inRepo(t, "tag", "-s", "-m", "release-right", "release-right")

	// Both tags reachable from main, so both are candidates.
	f.inRepo(t, "checkout", "-q", "main")
	f.inRepo(t, "merge", "-q", "--no-ff", "--no-gpg-sign", "-m", "merge", "left", "right")

	after, err := src.Select(ctx(t))
	if err != nil && !after.FellBack {
		// Either shape is acceptable, so long as neither tag was chosen.
		reason, ok := source.ReasonOf(err)
		if !ok || reason != source.AmbiguousTag {
			t.Fatalf("Select: %v", err)
		}
		return
	}
	if !after.FellBack {
		t.Fatalf("an ambiguous tag pair was chosen between, landing on %s", after.Revision)
	}
	if after.Revision != base {
		t.Errorf("reconciling %s, want to stay on %s", after.Revision, base)
	}
	if got := swappiness(t, after); got != "10" {
		t.Errorf("tree holds %q, want the base revision's 10", got)
	}
}

// A tag signed by a key the fleet does not trust is not a candidate, and one untrusted
// tag must not stop the host applying a legitimate one.
func TestATagSignedByAnUntrustedKeyIsNotACandidate(t *testing.T) {
	f := newFleet(t)
	f.commit(t, "base", true)
	f.inRepo(t, "tag", "-s", "-m", "release-1", "release-1")

	// A second key the fleet does not trust, used to tag a later commit.
	other := filepath.Join(t.TempDir(), "other")
	out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-C", "attacker@example.com",
		"-f", other).CombinedOutput()
	if err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	f.write(t, "layers/base/sysctl.yaml", sysctlDoc("99"))
	f.inRepo(t, "add", "-A")
	f.inRepo(t, "-c", "user.signingkey="+other, "commit", "-S", "-m", "attacker")
	f.inRepo(t, "-c", "user.signingkey="+other, "tag", "-s", "-m", "release-2", "release-2")

	sel, err := f.source(t, config.RequireSignedTag).Select(ctx(t))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if sel.Tag != "release-1" {
		t.Errorf("selected tag %q, want release-1", sel.Tag)
	}
	if got := swappiness(t, sel); got != "10" {
		t.Errorf("tree holds %q, want the trusted tag's 10", got)
	}
}

// The tree is replaced, not updated, or a document deleted in a newer revision would
// survive from the older one.
func TestTheMaterialisedTreeDoesNotKeepDeletedFiles(t *testing.T) {
	f := newFleet(t)
	f.write(t, "layers/base/extra.yaml", sysctlDoc("1"))
	f.commit(t, "with extra", true)

	src := f.source(t, config.RequireNone)
	sel, err := src.Select(ctx(t))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sel.Fleet, "layers", "base", "extra.yaml")); err != nil {
		t.Fatalf("the file is not in the first tree: %v", err)
	}

	f.inRepo(t, "rm", "-q", "layers/base/extra.yaml")
	f.commit(t, "without extra", true)

	after, err := src.Select(ctx(t))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if _, err := os.Stat(filepath.Join(after.Fleet, "layers", "base", "extra.yaml")); err == nil {
		t.Error("a deleted document survived in the materialised tree")
	}
}

// A fleet in a subdirectory is the ordinary monorepo case.
func TestAFleetInASubdirectoryIsFound(t *testing.T) {
	f := newFleet(t)
	// Moved before the first commit, since newFleet only writes the files.
	for _, name := range []string{"fleet.yaml", "hosts", "layers"} {
		if err := os.RemoveAll(filepath.Join(f.dir, name)); err != nil {
			t.Fatalf("remove: %v", err)
		}
	}
	f.write(t, "infra/fleet/fleet.yaml", fleetDoc)
	f.write(t, "infra/fleet/hosts/web-001.yaml", hostDoc("web-001"))
	f.write(t, "infra/fleet/layers/base/layer.yaml", layerDoc())
	f.write(t, "infra/fleet/layers/base/sysctl.yaml", sysctlDoc("10"))
	f.write(t, "README.md", "a repository holding more than a fleet\n")
	f.commit(t, "monorepo", true)

	withPrefix := source.New(f.config(config.RequireNone), "infra/fleet")
	sel, err := withPrefix.Select(ctx(t))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sel.Fleet, "fleet.yaml")); err != nil {
		t.Errorf("the fleet subdirectory was not found: %v", err)
	}
}
