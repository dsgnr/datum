// SPDX-License-Identifier: Apache-2.0

// Package posix implements File, Directory and Symlink.
//
// Reading is portable. Applying is not, because the safety rules need
// descriptor-relative syscalls, so it is only compiled on Linux. Elsewhere the
// observe, diff and plan half still works.
package posix

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strconv"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/state"
)

// Provider reads and writes files, directories and symbolic links.
type Provider struct{}

func New() Provider { return Provider{} }

func (Provider) Name() string { return "posix-file" }

func (Provider) Types() []string { return []string{"File", "Directory", "Symlink"} }

func (p Provider) Observe(ctx context.Context, req provider.Request) (provider.Observation, error) {
	if err := ctx.Err(); err != nil {
		return provider.Observation{}, err
	}

	// Lstat, so a link is reported as a link and not as what it points at.
	info, err := os.Lstat(req.Target)
	switch {
	case os.IsNotExist(err):
		return p.absent(req)
	case err != nil:
		return provider.Observation{}, err
	}

	// The wrong kind at the target reads as absent plus what was found, so a plan can say
	// occupied instead of listing every field as wrong.
	if kind := kindOf(info); kind != req.Ref.Type {
		observation, err := p.absent(req)
		if err != nil {
			return observation, err
		}
		observation.Found = describe(kind)
		return observation, nil
	}

	observation := provider.Observation{
		Exists: true,
		Fields: map[string]document.Value{},
	}

	owner, group, err := ownership(info)
	if err != nil {
		// Unobservable, not guessed at, or it would be false drift.
		observation.Unobservable = append(observation.Unobservable, "owner", "group")
	} else {
		observation.Fields["owner"] = document.Scalar(owner)
		observation.Fields["group"] = document.Scalar(group)
	}

	// A link's mode is ignored by the kernel, so the type has no such field.
	if req.Ref.Type != "Symlink" {
		observation.Fields["mode"] = document.Scalar(modeText(info.Mode()))
	}

	switch req.Ref.Type {
	case "Symlink":
		target, err := os.Readlink(req.Target)
		if err != nil {
			return observation, err
		}
		observation.Fields["target"] = document.Scalar(target)

	case "File":
		if err := p.compareContent(req, &observation); err != nil {
			return observation, err
		}
	}

	return observation, nil
}

// absent builds the observation for a missing target. The desired digest is still
// worked out, so a plan can say the content differs.
func (p Provider) absent(req provider.Request) (provider.Observation, error) {
	observation := provider.Observation{Exists: false, Fields: map[string]document.Value{}}
	if req.Ref.Type != "File" {
		return observation, nil
	}
	if err := p.compareContent(req, &observation); err != nil {
		return observation, err
	}
	return observation, nil
}

// compareContent digests both sides of a file's content, which is what makes text
// or a repository path comparable with bytes on disk.
func (p Provider) compareContent(req provider.Request, observation *provider.Observation) error {
	declared, kind, err := desiredContent(req)
	switch {
	case err != nil:
		return err
	case kind == contentNone:
		// Metadata only, so there is nothing to compare.
		return nil
	case kind == contentSecret:
		// Resolved at apply time and never in the manifest, so not comparable.
		observation.Unobservable = append(observation.Unobservable, "content")
		return nil
	}

	observation.Desired = document.Value{Kind: document.KindMap, Map: map[string]document.Value{
		"content": document.Scalar(provider.Digest(declared)),
	}}
	for name, value := range req.Desired.Map {
		if name != "content" {
			observation.Desired.Map[name] = value
		}
	}

	if !observation.Exists {
		return nil
	}
	actual, err := os.ReadFile(req.Target)
	if err != nil {
		return err
	}
	observation.Fields["content"] = document.Scalar(provider.Digest(actual))
	return nil
}

func (Provider) Apply(ctx context.Context, req provider.Request, action state.Action) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch action {
	case state.None, state.Skip:
		return nil
	case state.Remove:
		return remove(req)
	case state.Create, state.Update:
		return write(req)
	}
	return fmt.Errorf("posix-file: unexpected action %s", action)
}

// contentKind is where a file's content comes from.
type contentKind int

const (
	contentNone contentKind = iota
	contentLiteral
	contentSource
	contentSecret
)

// desiredContent reads the content a file should hold.
func desiredContent(req provider.Request) ([]byte, contentKind, error) {
	if text, ok := req.Field("content"); ok {
		return []byte(text), contentLiteral, nil
	}
	if _, ok := req.Field("secretRef"); ok {
		return nil, contentSecret, nil
	}

	// Templates are rendered during resolution, so by now the only difference
	// between source and template is which field named the file.
	for _, field := range []string{"source", "template"} {
		rel, ok := req.Field(field)
		if !ok {
			continue
		}
		data, err := req.ReadSource(rel)
		if err != nil {
			return nil, contentSource, fmt.Errorf("reading %s: %w", field, err)
		}
		return data, contentSource, nil
	}
	return nil, contentNone, nil
}

// kindOf maps what is on disk to the type that manages it.
func kindOf(info fs.FileInfo) string {
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		return "Symlink"
	case info.IsDir():
		return "Directory"
	case info.Mode().IsRegular():
		return "File"
	default:
		return "other"
	}
}

func describe(kind string) string {
	switch kind {
	case "Symlink":
		return "a symbolic link"
	case "Directory":
		return "a directory"
	case "File":
		return "a regular file"
	default:
		return "something that is not a file, directory or link"
	}
}

// modeText renders permission bits as a document writes them, including setuid,
// setgid and sticky.
func modeText(mode fs.FileMode) string {
	bits := mode.Perm()
	if mode&fs.ModeSetuid != 0 {
		bits |= 0o4000
	}
	if mode&fs.ModeSetgid != 0 {
		bits |= 0o2000
	}
	if mode&fs.ModeSticky != 0 {
		bits |= 0o1000
	}
	return "0" + strconv.FormatUint(uint64(bits), 8)
}

// parseMode reads a mode. Validation has already checked the shape.
func parseMode(text string) (fs.FileMode, error) {
	bits, err := strconv.ParseUint(text, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("mode %q is not octal", text)
	}
	return fs.FileMode(bits), nil
}
