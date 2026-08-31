//go:build integration

// SPDX-License-Identifier: Apache-2.0

// These tests drive the real git binary against real repositories with real signatures.
// What is under test is whether git reports a signature as good and made by a trusted
// key, so a fake returning "G" would pass without verifying anything.
//
// Signing uses ssh keys, which need no agent, keyring or expiry handling. The
// allowed-signers file is the trust root for both ssh and gpg.
package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsgnr/datum/internal/git"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return c
}

func requireTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"git", "ssh-keygen"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
}

// origin is a repository on disk that acts as the remote, plus the key material a fleet
// would hold.
type origin struct {
	dir     string
	signers string
	keyFile string
	homeDir string
}

func newOrigin(t *testing.T) *origin {
	t.Helper()
	requireTools(t)

	base := t.TempDir()
	o := &origin{
		dir:     filepath.Join(base, "origin"),
		signers: filepath.Join(base, "allowed-signers"),
		keyFile: filepath.Join(base, "id_ed25519"),
		homeDir: filepath.Join(base, "home"),
	}
	if err := os.MkdirAll(o.homeDir, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-C", "fleet@example.com",
		"-f", o.keyFile).CombinedOutput()
	if err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	pub, err := os.ReadFile(o.keyFile + ".pub")
	if err != nil {
		t.Fatalf("reading public key: %v", err)
	}
	// The allowed-signers format is an identity, then the key. The identity has to
	// match the committer for git to accept the signature as trusted.
	if err := os.WriteFile(o.signers, []byte("fleet@example.com "+string(pub)), 0o644); err != nil {
		t.Fatalf("writing signers: %v", err)
	}

	o.run(t, "init", "--initial-branch=main", o.dir)
	o.inRepo(t, "config", "user.name", "Fleet")
	o.inRepo(t, "config", "user.email", "fleet@example.com")
	o.inRepo(t, "config", "gpg.format", "ssh")
	o.inRepo(t, "config", "user.signingkey", o.keyFile)
	o.inRepo(t, "config", "gpg.ssh.allowedSignersFile", o.signers)
	return o
}

func (o *origin) run(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "HOME="+o.homeDir, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (o *origin) inRepo(t *testing.T, args ...string) string {
	t.Helper()
	return o.run(t, append([]string{"-C", o.dir}, args...)...)
}

// commit writes a file and commits it, signed or not.
func (o *origin) commit(t *testing.T, name, content string, signed bool) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(o.dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	o.inRepo(t, "add", name)
	args := []string{"commit", "-m", "add " + name}
	if signed {
		args = append(args, "-S")
	} else {
		args = append(args, "--no-gpg-sign")
	}
	o.inRepo(t, args...)
	return o.inRepo(t, "rev-parse", "HEAD")
}

func (o *origin) tag(t *testing.T, name string, signed bool) {
	t.Helper()
	if signed {
		o.inRepo(t, "tag", "-s", "-m", name, name)
		return
	}
	o.inRepo(t, "tag", name)
}

func (o *origin) client(t *testing.T) *git.Client {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone")
	c := git.New(clone, git.Limits{FetchTimeout: time.Minute})
	if err := c.EnsureClone(ctx(t), o.dir, "main"); err != nil {
		t.Fatalf("EnsureClone: %v", err)
	}
	return c
}

func TestCloneAndFetchKeepHistoryComplete(t *testing.T) {
	o := newOrigin(t)
	first := o.commit(t, "a", "one", true)

	c := o.client(t)
	shallow, err := c.IsShallow(ctx(t))
	if err != nil {
		t.Fatalf("IsShallow: %v", err)
	}
	if shallow {
		t.Error("the clone is shallow")
	}
	if err := c.RequireCompleteHistory(ctx(t)); err != nil {
		t.Errorf("RequireCompleteHistory: %v", err)
	}

	// A commit made after the clone arrives on a fetch, with no re-clone needed.
	second := o.commit(t, "b", "two", true)
	if err := c.Fetch(ctx(t)); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for _, rev := range []string{first, second} {
		has, err := c.Has(ctx(t), rev)
		if err != nil || !has {
			t.Errorf("Has(%s) = %t, %v", rev, has, err)
		}
	}
}

// A shallow clone answers ancestry questions wrongly rather than refusing, so it has to
// be caught before any control relies on it.
func TestAShallowCloneIsRefused(t *testing.T) {
	o := newOrigin(t)
	o.commit(t, "a", "one", true)
	o.commit(t, "b", "two", true)
	o.commit(t, "c", "three", true)

	shallow := filepath.Join(t.TempDir(), "shallow")
	o.run(t, "clone", "--depth", "1", "--no-local", "file://"+o.dir, shallow)

	c := git.New(shallow, git.Limits{})
	isShallow, err := c.IsShallow(ctx(t))
	if err != nil {
		t.Fatalf("IsShallow: %v", err)
	}
	if !isShallow {
		t.Fatal("a depth-1 clone was not reported as shallow")
	}
	if err := c.RequireCompleteHistory(ctx(t)); err == nil {
		t.Error("a shallow clone was accepted")
	}
}

func TestAncestryIsComputedFromRealHistory(t *testing.T) {
	o := newOrigin(t)
	first := o.commit(t, "a", "one", true)
	second := o.commit(t, "b", "two", true)

	c := o.client(t)
	cases := []struct {
		ancestor, descendant string
		want                 bool
	}{
		{first, second, true},
		{second, first, false},
		// A commit is its own ancestor, which is what makes reconciling an unchanged
		// revision not a downgrade.
		{first, first, true},
	}
	for _, tc := range cases {
		got, err := c.IsAncestor(ctx(t), tc.ancestor, tc.descendant)
		if err != nil {
			t.Fatalf("IsAncestor: %v", err)
		}
		if got != tc.want {
			t.Errorf("IsAncestor(%.7s, %.7s) = %t, want %t", tc.ancestor, tc.descendant, got, tc.want)
		}
	}
}

func TestASignedCommitVerifies(t *testing.T) {
	o := newOrigin(t)
	rev := o.commit(t, "a", "one", true)

	c := o.client(t)
	signature, err := c.VerifyCommit(ctx(t), rev, o.signers)
	if err != nil {
		t.Fatalf("VerifyCommit: %v", err)
	}
	if signature.KeyID == "" {
		t.Error("no key was reported, so the signer metric would be empty")
	}
	if signature.Signer != "fleet@example.com" {
		t.Errorf("signer = %q", signature.Signer)
	}
}

func TestAnUnsignedCommitIsRefused(t *testing.T) {
	o := newOrigin(t)
	rev := o.commit(t, "a", "one", false)

	c := o.client(t)
	if _, err := c.VerifyCommit(ctx(t), rev, o.signers); err == nil {
		t.Fatal("an unsigned commit verified")
	} else if !strings.Contains(err.Error(), "not signed") {
		t.Errorf("error = %v", err)
	}
}

// The case that matters most is a real signature from a key the fleet does not trust. A
// verifier that only checked the signature was well formed would accept this.
func TestACommitSignedByAnUntrustedKeyIsRefused(t *testing.T) {
	o := newOrigin(t)
	rev := o.commit(t, "a", "one", true)

	// An empty signers file means the key that signed is not among the trusted ones.
	empty := filepath.Join(t.TempDir(), "allowed-signers")
	if err := os.WriteFile(empty, []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := o.client(t)
	if _, err := c.VerifyCommit(ctx(t), rev, empty); err == nil {
		t.Fatal("a commit signed by an untrusted key verified")
	}
}

func TestASignedTagVerifiesAndAnUnsignedOneDoesNot(t *testing.T) {
	o := newOrigin(t)
	o.commit(t, "a", "one", true)
	o.tag(t, "release-2026.02.03", true)
	o.commit(t, "b", "two", true)
	o.tag(t, "lightweight", false)

	c := o.client(t)
	if err := c.Fetch(ctx(t)); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	signature, err := c.VerifyTag(ctx(t), "release-2026.02.03", o.signers)
	if err != nil {
		t.Fatalf("VerifyTag: %v", err)
	}
	if signature.KeyID == "" {
		t.Error("no key was reported for the tag")
	}
	if _, err := c.VerifyTag(ctx(t), "lightweight", o.signers); err == nil {
		t.Error("a lightweight tag verified")
	}
}

// A lightweight tag has nothing to sign, so it cannot be a candidate however it is
// named.
func TestOnlyAnnotatedTagsAreCandidates(t *testing.T) {
	o := newOrigin(t)
	o.commit(t, "a", "one", true)
	o.tag(t, "release-annotated", true)
	o.tag(t, "release-lightweight", false)
	o.tag(t, "other-annotated", true)

	c := o.client(t)
	if err := c.Fetch(ctx(t)); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	tags, err := c.AnnotatedTags(ctx(t), "release-*")
	if err != nil {
		t.Fatalf("AnnotatedTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "release-annotated" {
		t.Fatalf("tags = %+v, want only release-annotated", tags)
	}
}

func TestTheNewestTagIsSelected(t *testing.T) {
	o := newOrigin(t)
	o.commit(t, "a", "one", true)
	o.tag(t, "release-1", true)
	o.commit(t, "b", "two", true)
	o.tag(t, "release-2", true)

	c := o.client(t)
	if err := c.Fetch(ctx(t)); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	tags, err := c.AnnotatedTags(ctx(t), "release-*")
	if err != nil {
		t.Fatalf("AnnotatedTags: %v", err)
	}

	selected, err := c.SelectTag(ctx(t), tags)
	if err != nil {
		t.Fatalf("SelectTag: %v", err)
	}
	if selected.Name != "release-2" {
		t.Errorf("selected %s, want release-2", selected.Name)
	}
}

// Tagging one commit twice is ordinary, and refusing it would stop a repository tagging
// a release again.
func TestTwoTagsOnOneCommitAreOneCandidate(t *testing.T) {
	o := newOrigin(t)
	o.commit(t, "a", "one", true)
	o.tag(t, "release-1", true)
	o.tag(t, "release-1-again", true)

	c := o.client(t)
	if err := c.Fetch(ctx(t)); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	tags, err := c.AnnotatedTags(ctx(t), "release-*")
	if err != nil {
		t.Fatalf("AnnotatedTags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("tags = %+v", tags)
	}
	if _, err := c.SelectTag(ctx(t), tags); err != nil {
		t.Errorf("two tags on one commit were refused: %v", err)
	}
}

// Two signed tags on diverged histories have no correct answer, and choosing either
// would be a downgrade for somebody.
func TestDivergedTagsAreRefusedRatherThanChosenBetween(t *testing.T) {
	o := newOrigin(t)
	base := o.commit(t, "a", "one", true)

	o.inRepo(t, "checkout", "-b", "left", base)
	o.commit(t, "left", "l", true)
	o.tag(t, "release-left", true)

	o.inRepo(t, "checkout", "-b", "right", base)
	o.commit(t, "right", "r", true)
	o.tag(t, "release-right", true)

	c := o.client(t)
	if err := c.Fetch(ctx(t)); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	tags, err := c.AnnotatedTags(ctx(t), "release-*")
	if err != nil {
		t.Fatalf("AnnotatedTags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("tags = %+v", tags)
	}

	_, err = c.SelectTag(ctx(t), tags)
	if err == nil {
		t.Fatal("diverged tags were silently chosen between")
	}
	// The error has to name both, or an operator cannot see which histories diverged.
	for _, want := range []string{"ambiguous", "release-left", "release-right"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

// A force-push that removes the commit a host is pinned to is the case the descendant
// check cannot answer, and the host has to notice instead of accepting anything.
func TestARewrittenHistoryLosesTheRecordedRevision(t *testing.T) {
	o := newOrigin(t)
	o.commit(t, "a", "one", true)
	doomed := o.commit(t, "b", "two", true)

	c := o.client(t)
	if has, _ := c.Has(ctx(t), doomed); !has {
		t.Fatal("the commit is not in the clone")
	}

	// Rewritten upstream, then fetched with pruning, which is what a force-push looks
	// like to an agent.
	o.inRepo(t, "reset", "--hard", "HEAD~1")
	o.commit(t, "c", "three", true)

	// Over file:// and not a path, because a local clone hardlinks the whole object store
	// and would copy the unreachable commit along with everything else, making this test
	// pass for the wrong reason.
	fresh := filepath.Join(t.TempDir(), "fresh")
	after := git.New(fresh, git.Limits{FetchTimeout: time.Minute})
	if err := after.EnsureClone(ctx(t), "file://"+o.dir, "main"); err != nil {
		t.Fatalf("EnsureClone: %v", err)
	}
	has, err := after.Has(ctx(t), doomed)
	if err != nil {
		t.Fatalf("Has: %v", err)
	}
	if has {
		t.Error("the rewritten-away commit is still present, so this test proves nothing")
	}
}

func TestCheckoutWritesTheTreeOfARevision(t *testing.T) {
	o := newOrigin(t)
	first := o.commit(t, "a", "one", true)
	second := o.commit(t, "a", "two", true)

	c := o.client(t)
	for rev, want := range map[string]string{first: "one", second: "two"} {
		dir := filepath.Join(t.TempDir(), "tree")
		if err := c.Checkout(ctx(t), rev, dir); err != nil {
			t.Fatalf("Checkout(%.7s): %v", rev, err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "a"))
		if err != nil {
			t.Fatalf("reading the checked-out file: %v", err)
		}
		if string(data) != want {
			t.Errorf("revision %.7s gave %q, want %q", rev, data, want)
		}
	}
}

// A repository larger than the limit is refused, so one commit cannot make every host
// in a fleet pay for an unreasonable object store.
func TestARepositoryLargerThanTheLimitIsRefused(t *testing.T) {
	o := newOrigin(t)
	o.commit(t, "a", strings.Repeat("padding\n", 20000), true)

	clone := filepath.Join(t.TempDir(), "clone")
	c := git.New(clone, git.Limits{FetchTimeout: time.Minute, MaxRepositorySize: 1024})
	if err := c.EnsureClone(ctx(t), o.dir, "main"); err != nil {
		t.Fatalf("EnsureClone: %v", err)
	}
	if err := c.CheckSize(); err == nil {
		t.Fatal("an oversized repository was accepted")
	} else if !strings.Contains(err.Error(), "maxRepositorySize") {
		t.Errorf("the error does not name the setting: %v", err)
	}
}

// A hook in a fetched repository is a script the client would otherwise run.
func TestAHookInTheRepositoryIsNotRun(t *testing.T) {
	o := newOrigin(t)
	o.commit(t, "a", "one", true)

	c := o.client(t)
	marker := filepath.Join(t.TempDir(), "hook-ran")
	hook := filepath.Join(c.Dir(), ".git", "hooks", "post-checkout")
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatalf("mkdir hooks: %v", err)
	}
	script := "#!/bin/sh\ntouch " + marker + "\n"
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatalf("writing hook: %v", err)
	}

	rev, err := c.Resolve(ctx(t), "origin/main")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := c.Checkout(ctx(t), rev, filepath.Join(t.TempDir(), "tree")); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a post-checkout hook ran")
	}
}

// A filter named by .gitattributes is a command the client runs while checking files
// out, which would be arbitrary execution before a document has been parsed.
//
// The protection is that a filter only runs when a driver of that name is configured,
// and nothing Datum does configures one. This checks both halves, that the attributes
// file in the tree causes no execution and that the clone Datum made defines no filter
// for it to find. Configuring a driver and expecting the checkout to ignore it would
// test a guarantee that cannot be made, since command-line config cannot override a
// driver whose name is unknown.
func TestAGitattributesFilterHasNoDriverToRun(t *testing.T) {
	o := newOrigin(t)
	marker := filepath.Join(t.TempDir(), "filter-ran")

	if err := os.WriteFile(filepath.Join(o.dir, ".gitattributes"), []byte("* filter=evil\n"), 0o644); err != nil {
		t.Fatalf("write attributes: %v", err)
	}
	o.inRepo(t, "add", ".gitattributes")
	o.inRepo(t, "commit", "-S", "-m", "attributes")
	rev := o.commit(t, "a", "one", true)

	c := o.client(t)
	tree := filepath.Join(t.TempDir(), "tree")
	if err := c.Checkout(ctx(t), rev, tree); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a gitattributes filter ran during checkout")
	}
	// The content arrives unchanged, which is what a missing driver does instead of
	// failing the checkout.
	data, err := os.ReadFile(filepath.Join(tree, "a"))
	if err != nil || string(data) != "one" {
		t.Errorf("checked-out content = %q, %v", data, err)
	}

	config, err := os.ReadFile(filepath.Join(c.Dir(), ".git", "config"))
	if err != nil {
		t.Fatalf("reading the clone config: %v", err)
	}
	if strings.Contains(string(config), "filter") {
		t.Errorf("the clone config defines a filter:\n%s", config)
	}
}

// The checkout must not disturb the clone, or the next pass starts from a tree somebody
// else moved.
func TestCheckoutLeavesTheCloneAlone(t *testing.T) {
	o := newOrigin(t)
	first := o.commit(t, "a", "one", true)
	o.commit(t, "a", "two", true)

	c := o.client(t)
	before := o.run(t, "-C", c.Dir(), "rev-parse", "HEAD")

	if err := c.Checkout(ctx(t), first, filepath.Join(t.TempDir(), "tree")); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if after := o.run(t, "-C", c.Dir(), "rev-parse", "HEAD"); after != before {
		t.Errorf("HEAD moved from %s to %s", before, after)
	}
	// And no temporary index is left in the state directory.
	entries, err := os.ReadDir(filepath.Dir(c.Dir()))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "datum-index-") {
			t.Errorf("%s was left behind", entry.Name())
		}
	}
}

func TestRefsThatCouldBeOptionsAreRefused(t *testing.T) {
	o := newOrigin(t)
	o.commit(t, "a", "one", true)
	c := o.client(t)

	for _, bad := range []string{"--upload-pack=touch /tmp/x", "-x", "main..other", "HEAD^", "a b", ""} {
		if _, err := c.Resolve(ctx(t), bad); err == nil {
			t.Errorf("Resolve(%q) was accepted", bad)
		}
	}
}

// The fingerprint has to match what ssh tooling prints, or a fleet cannot match the
// metric against the key it is rotating to.
func TestSignersReportsTheSameFingerprintAsSshKeygen(t *testing.T) {
	o := newOrigin(t)

	out, err := exec.Command("ssh-keygen", "-l", "-f", o.keyFile+".pub").Output()
	if err != nil {
		t.Fatalf("ssh-keygen -l: %v", err)
	}
	// The output is `bits SHA256:... comment (TYPE)`.
	var want string
	for _, field := range strings.Fields(string(out)) {
		if strings.HasPrefix(field, "SHA256:") {
			want = field
		}
	}
	if want == "" {
		t.Fatalf("no fingerprint in %q", out)
	}

	signers, err := git.Signers(o.signers)
	if err != nil {
		t.Fatalf("Signers: %v", err)
	}
	if len(signers) != 1 {
		t.Fatalf("Signers = %+v", signers)
	}
	if signers[0].KeyID != want {
		t.Errorf("KeyID = %q, want %q", signers[0].KeyID, want)
	}
	if signers[0].Principals != "fleet@example.com" {
		t.Errorf("Principals = %q", signers[0].Principals)
	}
}

// A keyring, not an allowed-signers file, is how a gpg fleet holds its trust root, and
// that is not a misconfiguration to fail over.
func TestSignersIgnoresLinesItCannotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signers")
	content := "# a comment\n\nnot-a-signer-line\nfleet@example.com ssh-ed25519 !!!notbase64!!!\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	signers, err := git.Signers(path)
	if err != nil {
		t.Fatalf("Signers: %v", err)
	}
	if len(signers) != 0 {
		t.Errorf("Signers = %+v, want none", signers)
	}
}
