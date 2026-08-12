// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SourcePath resolves a repository-relative path against the layer that declared it and
// confirms it stays inside the checkout.
//
// Without the containment check, write access to one layer would be read access to
// anything the agent can reach. It lives here instead of in a provider because more
// than one type takes a repository path, and a check reimplemented per provider
// eventually differs.
func (r Request) SourcePath(rel string) (string, error) {
	// Both sides canonicalised, or the comparison is wrong wherever a parent
	// directory is itself a link.
	root, err := canonical(r.RepoRoot)
	if err != nil {
		return "", err
	}
	full := filepath.Join(root, filepath.FromSlash(r.LayerDir), filepath.FromSlash(rel))

	// Resolved before the check, so a link pointing out of the fleet is rejected.
	resolved, err := canonical(full)
	if err != nil {
		return "", err
	}
	if !withinRoot(root, resolved) {
		return "", fmt.Errorf("source %s resolves outside the repository", rel)
	}
	return resolved, nil
}

// ReadSource reads a repository-relative file.
func (r Request) ReadSource(rel string) ([]byte, error) {
	path, err := r.SourcePath(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// Digest is how content is compared when the two sides are not the same shape. A
// declared repository path and the bytes on a host only compare as digests.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// canonical resolves a path as far as it exists. A missing final component is not an
// error, because the read that follows gives a better message than this could.
func canonical(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved, nil
	}
	// Resolve the deepest part that exists, then put the tail back.
	dir, name := filepath.Split(absolute)
	resolvedDir, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return absolute, nil
	}
	return filepath.Join(resolvedDir, name), nil
}

func withinRoot(root, path string) bool {
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}
