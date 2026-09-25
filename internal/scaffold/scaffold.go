// SPDX-License-Identifier: Apache-2.0

// Package scaffold holds the documents a new repository starts with.
//
// The content lives here rather than in the command so that it can be checked
// against the parser in a test, which is what keeps a generated repository from
// being one that Datum then refuses to read.
package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dsgnr/datum/internal/document"
)

// File is one document the scaffold writes, with its path relative to the
// directory init was pointed at.
type File struct {
	Path string
	Body string
}

// Files returns the documents to write for a fleet of this name.
//
// The default is two files. Everything past them grows from use, and a generated
// repository is the first thing a new user reads, so what it contains teaches the
// habits they start with.
func Files(fleet string, withExamples bool) []File {
	files := []File{
		{
			Path: "fleet/datum.yaml",
			Body: fmt.Sprintf(`# Marks the root of the fleet. Everything below this directory is searched
# for Host, Layer and resource documents.
datum: %s
type: Fleet

name: %s
`, document.SchemaVersion, fleet),
		},
		{
			Path: "fleet/base/layer.yaml",
			Body: fmt.Sprintf(`# A layer with no matcher applies to every host in the fleet.
# Resource documents in this directory belong to this layer.
datum: %s
type: Layer

name: base

precedence: 0
`, document.SchemaVersion),
		},
	}
	if !withExamples {
		return files
	}
	return append(files,
		File{
			Path: "fleet/hosts/example-host.yaml",
			Body: fmt.Sprintf(`# A Host document declares a machine and classifies it. It carries no desired
# state of its own, and the labels are what layers match on.
datum: %s
type: Host

name: example-host

labels:
  role: web
  environment: production
`, document.SchemaVersion),
		},
		File{
			Path: "fleet/base/packages.yaml",
			Body: fmt.Sprintf(`# A resource document belongs to the nearest Layer document at or above it,
# which for this file is the base layer alongside it.
datum: %s
type: Package

name: curl

desired:
  state: present
`, document.SchemaVersion),
		},
	)
}

// Write creates each file under dir, and reports the paths it created.
//
// An existing file stops the whole thing before anything is written. Adding Datum
// to a repository that already has some is a mistake worth reporting rather than a
// reason to overwrite desired state somebody wrote.
func Write(dir string, files []File) ([]string, error) {
	for _, file := range files {
		full := filepath.Join(dir, filepath.FromSlash(file.Path))
		if _, err := os.Lstat(full); err == nil {
			return nil, fmt.Errorf("%s already exists, so nothing was written", file.Path)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("check %s: %w", file.Path, err)
		}
	}

	for _, file := range files {
		full := filepath.Join(dir, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, fmt.Errorf("create directory for %s: %w", file.Path, err)
		}
	}

	var written []string
	for _, file := range files {
		full := filepath.Join(dir, filepath.FromSlash(file.Path))
		out, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return nil, rollback(dir, written, fmt.Errorf("create %s: %w", file.Path, err))
		}
		written = append(written, file.Path)
		if _, err := out.Write([]byte(file.Body)); err != nil {
			if closeErr := out.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
			return nil, rollback(dir, written, fmt.Errorf("write %s: %w", file.Path, err))
		}
		if err := out.Close(); err != nil {
			return nil, rollback(dir, written, fmt.Errorf("close %s: %w", file.Path, err))
		}
	}
	return written, nil
}

func rollback(dir string, paths []string, cause error) error {
	for i := len(paths) - 1; i >= 0; i-- {
		full := filepath.Join(dir, filepath.FromSlash(paths[i]))
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			cause = errors.Join(cause, fmt.Errorf("remove incomplete file %s: %w", paths[i], err))
		}
	}
	return cause
}
