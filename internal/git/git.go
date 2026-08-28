// SPDX-License-Identifier: Apache-2.0

// Package git obtains repository history and answers questions about it.
//
// Submodules, `.gitattributes` filters, repository-local config and hooks all cause the
// client to execute programs or reach the network on the repository's instruction. All
// four are disabled here.
//
// The clone is kept complete, including tags. The descendant check and signed-tag
// selection both require ancestry, which a shallow clone computes incorrectly rather
// than reporting as unavailable.
package git

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsgnr/datum/internal/run"
)

// hardening goes on every invocation, not into the clone's config, so a repository
// shipping its own config cannot turn any of it off.
var hardening = []string{
	// A hook in a fetched repository is a script the client would run.
	"-c", "core.hooksPath=/dev/null",
	// Neither an ambient attributes file nor one in the tree may name a filter.
	"-c", "core.attributesFile=/dev/null",
	// A submodule is a second repository fetched from an address the first one
	// chooses, so it is never followed.
	"-c", "submodule.recurse=false",
	"-c", "fetch.recurseSubmodules=no",
	// The ext transport names a command for git to run, which is arbitrary execution
	// chosen by a URL.
	"-c", "protocol.ext.allow=never",
	// Set explicitly even though it is git's default today. A file URL is fine for the
	// configured source, a local mirror say, and dangerous only for a submodule. That is
	// what "user" buys, allowing it when this process asked for it and refusing it when a
	// repository did.
	"-c", "protocol.file.allow=user",
	// A credential helper is a command the client runs. The credential is a file this
	// package passes in, and nothing should be asked for interactively.
	"-c", "credential.helper=",
	// Background repacking during a pass competes with the pass for io.
	"-c", "gc.auto=0",
	"-c", "maintenance.auto=false",
	"-c", "advice.detachedHead=false",
}

// environment keeps git away from any configuration this process did not choose. The
// runner inherits nothing, so these are the whole environment git sees.
var environment = map[string]string{
	"GIT_CONFIG_NOSYSTEM": "1",
	"GIT_CONFIG_GLOBAL":   "/dev/null",
	"GIT_CONFIG_SYSTEM":   "/dev/null",
	// A prompt in a pass is a hang. A missing credential should fail, not wait for a
	// terminal that is not there.
	"GIT_TERMINAL_PROMPT": "0",
	"GIT_ASKPASS":         "",
	// Placed inside the state directory by the caller where it matters. Set to
	// something unwritable by default so a stray read cannot find a real home.
	"HOME": "/nonexistent",
}

// Limits bound what a single commit can make a whole fleet pay for.
type Limits struct {
	// FetchTimeout bounds the network operation.
	FetchTimeout time.Duration
	// MaxRepositorySize bounds what the agent accepts into its object store, which
	// protects against a repository that has grown unreasonably as much as against a
	// deliberate one.
	MaxRepositorySize int64
}

// Client works with one local clone.
type Client struct {
	runner run.Runner
	dir    string
	limits Limits
}

// New returns a client for a clone at dir.
func New(dir string, limits Limits) *Client {
	return NewWith(run.Exec{Env: environment}, dir, limits)
}

// NewWith returns a client driving a supplied runner.
func NewWith(runner run.Runner, dir string, limits Limits) *Client {
	return &Client{runner: runner, dir: dir, limits: limits}
}

// Dir is the clone directory.
func (c *Client) Dir() string { return c.dir }

// Available reports whether git is installed.
func Available() bool { return run.Available("git") }

// Exists reports whether the clone is already there.
func (c *Client) Exists() bool {
	info, err := os.Stat(filepath.Join(c.dir, ".git"))
	return err == nil && info.IsDir()
}

// git runs a command inside the clone.
func (c *Client) git(ctx context.Context, args ...string) (run.Result, error) {
	argv := append([]string{"git"}, hardening...)
	argv = append(argv, "-C", c.dir)
	return c.runner.Run(ctx, append(argv, args...)...)
}

// gitOutside runs a command that cannot be inside the clone, such as the clone itself.
func (c *Client) gitOutside(ctx context.Context, args ...string) (run.Result, error) {
	argv := append([]string{"git"}, hardening...)
	return c.runner.Run(ctx, append(argv, args...)...)
}

// text returns trimmed stdout, or an error describing what git said.
func text(result run.Result, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if !result.OK() {
		return "", result.Err()
	}
	return strings.TrimSpace(result.Stdout), nil
}

// EnsureClone creates the clone if it is absent and leaves it alone otherwise.
//
// A full clone including tags, because fetching only some tags would make an agent
// pick a different revision from one that fetched them all.
func (c *Client) EnsureClone(ctx context.Context, url, branch string) error {
	if c.Exists() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.dir), 0o700); err != nil {
		return err
	}

	ctx, cancel := c.withFetchTimeout(ctx)
	defer cancel()

	args := []string{"clone", "--tags", "--no-recurse-submodules", "--no-checkout"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	// The separator matters because a URL beginning with a dash would otherwise be
	// read as an option.
	args = append(args, "--", url, c.dir)

	result, err := c.gitOutside(ctx, args...)
	if err != nil {
		return fmt.Errorf("cloning %s: %w", url, err)
	}
	if !result.OK() {
		// Removed so a half-made clone is not mistaken for a usable one on the next
		// pass, which would then fetch into it and report a confusing error.
		os.RemoveAll(c.dir)
		return fmt.Errorf("cloning %s: %w", url, result.Err())
	}
	return nil
}

// Fetch updates the clone. Fetching instead of re-cloning keeps history and tags
// complete across passes.
func (c *Client) Fetch(ctx context.Context) error {
	ctx, cancel := c.withFetchTimeout(ctx)
	defer cancel()

	// Tags are pruned along with branches, because a tag deleted upstream should stop
	// being a candidate, not linger as one only this host can see.
	result, err := c.git(ctx, "fetch", "--tags", "--prune", "--prune-tags",
		"--no-recurse-submodules", "origin")
	if err != nil {
		return fmt.Errorf("fetching: %w", err)
	}
	if !result.OK() {
		return fmt.Errorf("fetching: %w", result.Err())
	}
	return c.CheckSize()
}

func (c *Client) withFetchTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.limits.FetchTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.limits.FetchTimeout)
}

// CheckSize refuses a clone larger than the configured limit.
//
// Measured after the fetch and not negotiated during it, because git has no dependable
// way to decline a transfer partway through. So the limit stops a host applying an
// unreasonable repository, not downloading one.
func (c *Client) CheckSize() error {
	if c.limits.MaxRepositorySize <= 0 {
		return nil
	}
	size, err := dirSize(filepath.Join(c.dir, ".git"))
	if err != nil {
		return err
	}
	if size > c.limits.MaxRepositorySize {
		return fmt.Errorf("the repository is %d bytes, which is larger than source.maxRepositorySize of %d",
			size, c.limits.MaxRepositorySize)
	}
	return nil
}

// IsShallow reports whether history is truncated.
func (c *Client) IsShallow(ctx context.Context) (bool, error) {
	out, err := text(c.git(ctx, "rev-parse", "--is-shallow-repository"))
	if err != nil {
		return false, err
	}
	return out == "true", nil
}

// RequireCompleteHistory refuses a clone that cannot answer an ancestry question.
func (c *Client) RequireCompleteHistory(ctx context.Context) error {
	shallow, err := c.IsShallow(ctx)
	if err != nil {
		return err
	}
	if shallow {
		return fmt.Errorf("the clone at %s is shallow, so revision ordering cannot be computed", c.dir)
	}
	return nil
}

// Resolve turns a ref into a full object name.
func (c *Client) Resolve(ctx context.Context, ref string) (string, error) {
	if err := validRef(ref); err != nil {
		return "", err
	}
	// The commit suffix resolves a tag to the commit it names, not the tag object, which
	// is what every ancestry question here is about.
	return text(c.git(ctx, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}"))
}

// Has reports whether a commit is present in the local object store.
func (c *Client) Has(ctx context.Context, rev string) (bool, error) {
	if err := validRef(rev); err != nil {
		return false, err
	}
	result, err := c.git(ctx, "cat-file", "-e", rev+"^{commit}")
	if err != nil {
		return false, err
	}
	return result.OK(), nil
}

// IsAncestor reports whether ancestor is reachable from descendant. A commit is its
// own ancestor, which is what makes reconciling an unchanged revision not a downgrade.
func (c *Client) IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	if err := validRef(ancestor); err != nil {
		return false, err
	}
	if err := validRef(descendant); err != nil {
		return false, err
	}
	result, err := c.git(ctx, "merge-base", "--is-ancestor", ancestor, descendant)
	if err != nil {
		return false, err
	}
	switch result.Code {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		// Anything else means git could not answer, such as a missing object, and
		// that is not the same as a no.
		return false, result.Err()
	}
}

// Checkout writes the tree of a revision into dir.
//
// Built through a temporary index instead of moving the clone's HEAD, so the clone
// stays where it was and a later operation sees nothing of this checkout. It also means
// two revisions can be materialised in one pass, which is what falling back needs.
//
// Symbolic links in the tree are written as links and never followed when a source is
// resolved, which the containment check on a repository path enforces separately.
func (c *Client) Checkout(ctx context.Context, rev, dir string) error {
	if err := validRef(rev); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	index, err := os.CreateTemp(filepath.Dir(dir), "datum-index-")
	if err != nil {
		return err
	}
	index.Close()
	defer os.Remove(index.Name())

	// read-tree fills the temporary index and checkout-index writes the files, which
	// together are the operation checkout performs without the side effects on HEAD.
	env := map[string]string{"GIT_INDEX_FILE": index.Name()}
	if result, err := c.gitEnv(ctx, env, "read-tree", "--end-of-options", rev); err != nil {
		return fmt.Errorf("reading the tree of %s: %w", rev, err)
	} else if !result.OK() {
		return fmt.Errorf("reading the tree of %s: %w", rev, result.Err())
	}

	result, err := c.gitEnv(ctx, env, "--work-tree", dir, "checkout-index", "--all", "--force")
	if err != nil {
		return fmt.Errorf("checking out %s: %w", rev, err)
	}
	if !result.OK() {
		return fmt.Errorf("checking out %s: %w", rev, result.Err())
	}
	return nil
}

// gitEnv runs a command with extra environment, which only the checkout needs.
func (c *Client) gitEnv(ctx context.Context, extra map[string]string, args ...string) (run.Result, error) {
	runner := c.runner
	if exec, ok := runner.(run.Exec); ok {
		runner = exec.With(extra)
	}
	argv := append([]string{"git"}, hardening...)
	argv = append(argv, "-C", c.dir)
	return runner.Run(ctx, append(argv, args...)...)
}

// validRef refuses anything that could be read as an option or smuggle a revision
// expression past a caller that meant a plain name.
func validRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("git: an empty ref")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("git: ref %q begins with a dash", ref)
	}
	for _, bad := range []string{" ", "\t", "\n", "..", "^", "~", ":", "?", "*", "[", "\\", "@{"} {
		if strings.Contains(ref, bad) {
			return fmt.Errorf("git: ref %q contains %q", ref, bad)
		}
	}
	return nil
}

func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A file that vanished during the walk is not a reason to refuse a
			// repository, and git rewrites pack files as it works.
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}
