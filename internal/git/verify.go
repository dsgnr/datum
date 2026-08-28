// SPDX-License-Identifier: Apache-2.0

package git

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Signature is what verification established about a revision.
type Signature struct {
	// KeyID identifies the key that signed, for the metric that exists so a fleet can
	// tell how far a key rotation has progressed.
	KeyID string
	// Signer is the identity the allowed-signers file associated with the key, where
	// the format carries one.
	Signer string
}

// signersConfig points git at the allowed-signers file.
//
// Both signing formats are configured, because which one a fleet uses is its own
// decision and the file name is the same either way. For ssh the file is the trust
// root, for gpg the keyring is, so only the ssh path is named here.
func signersConfig(signers string) []string {
	if signers == "" {
		return nil
	}
	return []string{"-c", "gpg.ssh.allowedSignersFile=" + signers}
}

// VerifyCommit checks that a commit carries a good signature from a trusted key.
//
// The status comes from git's own verification, not from parsing a signature, because
// git knows which trust root applies to which format.
func (c *Client) VerifyCommit(ctx context.Context, rev, signers string) (Signature, error) {
	if err := validRef(rev); err != nil {
		return Signature{}, err
	}

	args := append(signersConfig(signers), "log", "-1",
		// A tab separator, because a signer identity can contain spaces.
		"--format=%G?%x09%GK%x09%GS", "--no-show-signature", "--end-of-options", rev)
	result, err := c.git(ctx, args...)
	if err != nil {
		return Signature{}, err
	}
	if !result.OK() {
		return Signature{}, result.Err()
	}

	fields := strings.SplitN(strings.TrimRight(result.Stdout, "\n"), "\t", 3)
	status := ""
	if len(fields) > 0 {
		status = strings.TrimSpace(fields[0])
	}
	signature := Signature{}
	if len(fields) > 1 {
		signature.KeyID = strings.TrimSpace(fields[1])
	}
	if len(fields) > 2 {
		signature.Signer = strings.TrimSpace(fields[2])
	}

	if err := commitStatusError(status, rev); err != nil {
		return Signature{}, err
	}
	return signature, nil
}

// commitStatusError turns git's one-letter verdict into something a report can carry.
//
// Only G counts. A signature that is good but from a key outside the trust root is U,
// and treating that as acceptable would make the allowed-signers file decorative.
func commitStatusError(status, rev string) error {
	switch status {
	case "G":
		return nil
	case "U":
		return fmt.Errorf("%s is signed by a key that is not in the trusted signers", rev)
	case "B":
		return fmt.Errorf("%s has a bad signature", rev)
	case "X":
		return fmt.Errorf("%s is signed by an expired key", rev)
	case "Y":
		return fmt.Errorf("%s is signed by a key that has expired", rev)
	case "R":
		return fmt.Errorf("%s is signed by a revoked key", rev)
	case "N", "":
		return fmt.Errorf("%s is not signed", rev)
	default:
		// E covers a signature git could not check at all, which usually means no
		// allowed-signers file or an unreadable one.
		return fmt.Errorf("%s could not be verified, which is usually a missing or unreadable signers file", rev)
	}
}

// keyPattern pulls a key out of what verify-tag reports. ssh names a fingerprint and
// gpg a long key id, and the exit code is the authority either way, so this is only for
// reporting which key was used.
var keyPattern = regexp.MustCompile(`(SHA256:[A-Za-z0-9+/=]+)|\b([0-9A-Fa-f]{16,40})\b`)

// VerifyTag checks that an annotated tag object carries a good signature.
//
// The signature is on the tag object, not the commit it points at, which is why
// signed-tag mode costs nothing per commit.
func (c *Client) VerifyTag(ctx context.Context, tag, signers string) (Signature, error) {
	if err := validRef(tag); err != nil {
		return Signature{}, err
	}

	args := append(signersConfig(signers), "verify-tag", "--raw", "--end-of-options", tag)
	result, err := c.git(ctx, args...)
	if err != nil {
		return Signature{}, err
	}
	if !result.OK() {
		return Signature{}, fmt.Errorf("tag %s does not carry a good signature from a trusted key", tag)
	}

	// Reported on stderr by both formats, which is where git writes verification
	// detail even on success.
	signature := Signature{}
	if match := keyPattern.FindStringSubmatch(result.Stderr); match != nil {
		if match[1] != "" {
			signature.KeyID = match[1]
		} else {
			signature.KeyID = match[2]
		}
	}
	signature.Signer = signerFrom(result.Stderr)
	return signature, nil
}

// signerFrom pulls the identity out of an ssh verification line, which reads
// `Good "git" signature for NAME with ...`.
var signerPattern = regexp.MustCompile(`signature for ([^\s]+) with`)

func signerFrom(stderr string) string {
	if match := signerPattern.FindStringSubmatch(stderr); match != nil {
		return match[1]
	}
	return ""
}

// Tag is an annotated tag and the commit it names.
type Tag struct {
	Name   string
	Commit string
}

// AnnotatedTags lists annotated tags whose name matches pattern.
//
// Lightweight tags are excluded because a lightweight tag is a bare reference with
// nothing to sign, so it cannot be a candidate however it is named.
func (c *Client) AnnotatedTags(ctx context.Context, pattern string) ([]Tag, error) {
	if pattern == "" {
		return nil, nil
	}
	if strings.HasPrefix(pattern, "-") {
		return nil, fmt.Errorf("git: tag pattern %q begins with a dash", pattern)
	}

	// objecttype tells a tag object from a commit, which is how a lightweight tag is
	// recognised. The dereferenced object name is the commit either way.
	out, err := text(c.git(ctx, "for-each-ref",
		"--format=%(objecttype)%09%(refname:short)%09%(objectname)%09%(*objectname)",
		"refs/tags/"+pattern))
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	var tags []Tag
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 4 || fields[0] != "tag" {
			continue
		}
		tags = append(tags, Tag{Name: fields[1], Commit: fields[3]})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })
	return tags, nil
}

// SelectTag picks the one candidate that descends from every other candidate.
//
// Two tags on the same commit count as one candidate, since neither is a strict
// descendant of the other. Where no unique candidate exists, because two signed tags
// sit on diverged histories, this returns an error naming both.
func (c *Client) SelectTag(ctx context.Context, candidates []Tag) (Tag, error) {
	switch len(candidates) {
	case 0:
		return Tag{}, fmt.Errorf("no signed tag is a candidate")
	case 1:
		return candidates[0], nil
	}

	for _, candidate := range candidates {
		descendsFromAll := true
		for _, other := range candidates {
			if other.Commit == candidate.Commit {
				continue
			}
			ok, err := c.IsAncestor(ctx, other.Commit, candidate.Commit)
			if err != nil {
				return Tag{}, err
			}
			if !ok {
				descendsFromAll = false
				break
			}
		}
		if descendsFromAll {
			return candidate, nil
		}
	}
	return Tag{}, ambiguous(candidates)
}

// ambiguous describes the tags that could not be ordered, so an operator can see which
// two histories diverged rather than being told only that something was ambiguous.
func ambiguous(candidates []Tag) error {
	var b strings.Builder
	b.WriteString("ambiguous signed tag, neither of these descends from the other:")
	for _, candidate := range candidates {
		short := candidate.Commit
		if len(short) > 7 {
			short = short[:7]
		}
		fmt.Fprintf(&b, "\n  %s   %s", candidate.Name, short)
	}
	return fmt.Errorf("%s", b.String())
}
