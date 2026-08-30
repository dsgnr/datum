// SPDX-License-Identifier: Apache-2.0

// Package source decides which revision a pass reconciles.
//
// A pass fetches, selects a candidate, verifies its signature against the trusted keys,
// checks it descends from the revision this host accepted, then materialises it. The
// order is fixed and each step can only reject a candidate.
//
// A candidate failing any step leaves the host on the revision it already accepted.
package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsgnr/datum/internal/config"
	"github.com/dsgnr/datum/internal/git"
	"github.com/dsgnr/datum/internal/revision"
)

// Refusal names the control that turned a revision away, using the reasons the metric
// catalogue already defines.
type Refusal string

const (
	Unsigned          Refusal = "unsigned"
	UntrustedSigner   Refusal = "untrusted-signer"
	AmbiguousTag      Refusal = "ambiguous-tag"
	NotDescendant     Refusal = "not-descendant"
	UnsupportedSchema Refusal = "unsupported-schema"
)

// RefusedError is a revision a control declined. Separate from an ordinary failure
// because a refusal is the control working, and it gets counted, not just logged.
type RefusedError struct {
	Reason   Refusal
	Revision string
	Err      error
}

func (e *RefusedError) Error() string { return e.Err.Error() }
func (e *RefusedError) Unwrap() error { return e.Err }

func refuse(reason Refusal, rev string, err error) error {
	return &RefusedError{Reason: reason, Revision: rev, Err: err}
}

// ReasonOf returns the control that refused, and false for anything else.
func ReasonOf(err error) (Refusal, bool) {
	var refused *RefusedError
	if errors.As(err, &refused) {
		return refused.Reason, true
	}
	return "", false
}

// Selection is the revision a pass should reconcile and how it was arrived at.
type Selection struct {
	// Revision is the full object name to reconcile.
	Revision string
	// Tree is the directory its content was written to.
	Tree string
	// Fleet is the fleet directory inside Tree, which differs when the fleet is a
	// subdirectory of the repository.
	Fleet string

	// Tag is the signed tag that named the revision, empty under signed-commit.
	Tag string
	// Signature identifies the key that vouched for it, empty under require: none.
	Signature git.Signature

	// Accepted is what the host had accepted before this pass.
	Accepted string
	// FirstContact is true when the host had no accepted revision, which is a genuine
	// trust-on-first-use gap that provisioning closes by writing a baseline.
	FirstContact bool
	// FellBack is true when the candidate was refused and this is the accepted
	// revision being reconciled again.
	FellBack bool
	// Candidate is the revision that was refused, empty where the refusal happened
	// before one could be identified, such as a fetch that failed.
	Candidate string
	// Why explains the fallback in a form a report can carry.
	Why string
	// Cause is the refusal itself, so a caller can count it by reason.
	Cause error
}

// Source obtains revisions for one agent.
type Source struct {
	cfg    config.Config
	client *git.Client
	// prefix is the fleet directory relative to the repository root, so a monorepo
	// resolves the right subdirectory of whatever tree is materialised.
	prefix string
}

// New returns a source backed by a clone inside the state directory.
//
// The clone lives beside the accepted revision on purpose. Both are state this host
// keeps between passes, and both are covered by the same directory permissions.
func New(cfg config.Config, prefix string) *Source {
	clone := filepath.Join(cfg.State, "repository")
	return NewWith(cfg, prefix, git.New(clone, git.Limits{
		FetchTimeout:      time.Duration(cfg.Source.FetchTimeout),
		MaxRepositorySize: int64(cfg.Source.MaxRepositorySize),
	}))
}

// NewWith returns a source using a supplied client.
func NewWith(cfg config.Config, prefix string, client *git.Client) *Source {
	return &Source{cfg: cfg, client: client, prefix: prefix}
}

// Client exposes the clone, for reporting and for tests.
func (s *Source) Client() *git.Client { return s.client }

// Select fetches and decides what to reconcile.
//
// An error means neither the candidate nor a fallback could be used, which is a host
// with nothing to do, not a host that chose to do nothing.
func (s *Source) Select(ctx context.Context) (Selection, error) {
	accepted, hasAccepted, err := revision.Read(s.cfg.State)
	if err != nil {
		return Selection{}, err
	}
	sel := Selection{Accepted: accepted, FirstContact: !hasAccepted}

	if err := s.client.EnsureClone(ctx, s.cfg.Source.URL, s.cfg.Source.Branch); err != nil {
		return s.fallback(ctx, sel, err)
	}
	if err := s.client.Fetch(ctx); err != nil {
		// A host that cannot reach its source keeps reconciling what it holds. That is
		// what makes Datum usable on machines that are not permanently connected.
		return s.fallback(ctx, sel, err)
	}
	// Checked after fetching, because a clone that was complete can only have become
	// shallow through something unusual, and ancestry is about to be relied on.
	if err := s.client.RequireCompleteHistory(ctx); err != nil {
		return s.fallback(ctx, sel, err)
	}

	candidate, tag, signature, err := s.candidate(ctx)
	if err != nil {
		return s.fallback(ctx, sel, err)
	}
	if err := s.checkCurrent(ctx, candidate, accepted, hasAccepted); err != nil {
		return s.fallback(ctx, sel, err)
	}

	tree, fleet, err := s.materialise(ctx, candidate)
	if err != nil {
		return s.fallback(ctx, sel, err)
	}

	sel.Revision = candidate
	sel.Tree = tree
	sel.Fleet = fleet
	sel.Tag = tag
	sel.Signature = signature
	return sel, nil
}

// candidate finds the revision this host would apply, before any ordering check.
func (s *Source) candidate(ctx context.Context) (string, string, git.Signature, error) {
	branch := s.cfg.Source.Branch
	switch s.cfg.Trust.Require {
	case config.RequireNone:
		// The branch tip, unverified. A fleet running this way shows up in a metric.
		rev, err := s.client.Resolve(ctx, "origin/"+branch)
		return rev, "", git.Signature{}, err

	case config.RequireSignedCommit:
		rev, err := s.client.Resolve(ctx, "origin/"+branch)
		if err != nil {
			return "", "", git.Signature{}, err
		}
		signature, err := s.client.VerifyCommit(ctx, rev, s.cfg.Trust.Signers)
		if err != nil {
			return "", "", git.Signature{}, s.signatureRefusal(rev, err)
		}
		return rev, "", signature, nil

	case config.RequireSignedTag:
		return s.signedTag(ctx)
	}
	return "", "", git.Signature{}, fmt.Errorf("unknown trust.require %q", s.cfg.Trust.Require)
}

// signatureRefusal separates a missing signature from one made by a key outside the
// trust root, because they are different failures with different fixes.
func (s *Source) signatureRefusal(rev string, err error) error {
	reason := Unsigned
	if isUntrusted(err) {
		reason = UntrustedSigner
	}
	return refuse(reason, rev, err)
}

func isUntrusted(err error) bool {
	message := err.Error()
	for _, phrase := range []string{"not in the trusted signers", "revoked", "expired"} {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

// signedTag applies the candidate rules and selects among them.
func (s *Source) signedTag(ctx context.Context) (string, string, git.Signature, error) {
	tags, err := s.client.AnnotatedTags(ctx, s.cfg.Trust.TagPattern)
	if err != nil {
		return "", "", git.Signature{}, err
	}

	accepted, hasAccepted, err := revision.Read(s.cfg.State)
	if err != nil {
		return "", "", git.Signature{}, err
	}

	var candidates []git.Tag
	signatures := map[string]git.Signature{}
	for _, tag := range tags {
		signature, err := s.client.VerifyTag(ctx, tag.Name, s.cfg.Trust.Signers)
		if err != nil {
			// Not a candidate, not an error. A repository may hold tags signed by keys this
			// fleet does not trust, and failing the pass over one would hand anybody who can
			// push a tag a fleet-wide outage.
			continue
		}
		reachable, err := s.client.IsAncestor(ctx, tag.Commit, "origin/"+s.cfg.Source.Branch)
		if err != nil || !reachable {
			// A tag on a branch nobody deploys is not a deployment candidate.
			continue
		}
		if hasAccepted {
			// Bounding by the accepted revision keeps selection computable on a
			// long-lived repository, and a tag older than the baseline could never be
			// applied anyway.
			newer, err := s.client.IsAncestor(ctx, accepted, tag.Commit)
			if err != nil || !newer {
				continue
			}
		}
		candidates = append(candidates, tag)
		signatures[tag.Name] = signature
	}

	if len(candidates) == 0 {
		return "", "", git.Signature{}, fmt.Errorf(
			"no signed tag matching %q is a candidate, so there is nothing newer to apply",
			s.cfg.Trust.TagPattern)
	}

	selected, err := s.client.SelectTag(ctx, candidates)
	if err != nil {
		return "", "", git.Signature{}, refuse(AmbiguousTag, "", err)
	}
	return selected.Commit, selected.Name, signatures[selected.Name], nil
}

// checkCurrent refuses a candidate that is not newer than what the host accepted.
func (s *Source) checkCurrent(ctx context.Context, candidate, accepted string, hasAccepted bool) error {
	if !hasAccepted {
		// First contact. There is nothing to compare against, which is a gap that
		// cannot be closed from the host.
		return nil
	}
	if !s.cfg.Trust.Descendant() {
		return nil
	}

	present, err := s.client.Has(ctx, accepted)
	if err != nil {
		return err
	}
	if !present {
		// Treating this as first contact would be convenient and would make the
		// control trivially defeatable, because an attacker who can force-push could
		// remove the commit a host is pinned to and have it accept anything.
		return refuse(NotDescendant, candidate, fmt.Errorf(
			"cannot verify revision ordering, because the accepted revision %s is not in the repository\n"+
				"  candidate          %s\n"+
				"  history appears to have been rewritten\n"+
				"  run `datum revision clear --yes` to accept a new baseline",
			short(accepted), short(candidate)))
	}

	descends, err := s.client.IsAncestor(ctx, accepted, candidate)
	if err != nil {
		return err
	}
	if !descends {
		return refuse(NotDescendant, candidate, fmt.Errorf(
			"%s does not descend from the accepted revision %s, so it would be a downgrade",
			short(candidate), short(accepted)))
	}
	return nil
}

// materialise writes a revision's tree where resolution can read it.
func (s *Source) materialise(ctx context.Context, rev string) (string, string, error) {
	tree := filepath.Join(s.cfg.State, "tree")
	// Replaced, not updated, so a file deleted in the new revision does not survive from
	// the old one.
	if err := os.RemoveAll(tree); err != nil {
		return "", "", err
	}
	if err := s.client.Checkout(ctx, rev, tree); err != nil {
		return "", "", err
	}
	return tree, s.fleetIn(tree), nil
}

func (s *Source) fleetIn(tree string) string {
	if s.prefix == "" {
		return tree
	}
	return filepath.Join(tree, filepath.FromSlash(s.prefix))
}

// fallback reconciles the accepted revision when the candidate cannot be used.
//
// A host with no accepted revision has nothing to fall back to, so the failure stands.
// That is the only case where a source problem leaves a host unmanaged, and it is a
// host that was never managed, not one that stopped being.
func (s *Source) fallback(ctx context.Context, sel Selection, cause error) (Selection, error) {
	if sel.FirstContact {
		return Selection{}, cause
	}

	tree, fleet, err := s.materialise(ctx, sel.Accepted)
	if err != nil {
		// Both the candidate and the fallback are unusable, so the fallback failure is the
		// one to report, not the original.
		return Selection{}, fmt.Errorf("%w, and the accepted revision %s could not be used either: %v",
			cause, short(sel.Accepted), err)
	}

	sel.Revision = sel.Accepted
	sel.Tree = tree
	sel.Fleet = fleet
	sel.FellBack = true
	sel.Cause = cause
	sel.Why = cause.Error()
	// The refusal reason is preserved so the caller can count it, even though the pass
	// continues against the older revision.
	var refused *RefusedError
	if errors.As(cause, &refused) {
		sel.Why = string(refused.Reason) + ": " + cause.Error()
		sel.Candidate = refused.Revision
	}
	return sel, nil
}

// Accept advances the pointer to a revision that resolved and validated.
//
// Called after resolution, not after apply, because a revision whose apply failed on
// some resource still advances. Otherwise one persistently failing resource pins a host
// at an old revision for ever, and the fix for it never arrives.
func (s *Source) Accept(rev string) error {
	return revision.Advance(s.cfg.State, rev)
}

func short(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}
