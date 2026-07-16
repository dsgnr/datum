//go:build linux

// SPDX-License-Identifier: Apache-2.0

package posix

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/dsgnr/datum/internal/provider"
)

// Everything here goes through a held directory descriptor, never a path string. A path
// re-resolved at write time can resolve somewhere else. A descriptor cannot.

// write creates or updates a target.
func write(req provider.Request) error {
	parent, name, err := openParent(req.Target)
	if err != nil {
		return err
	}
	defer parent.Close()

	switch req.Ref.Type {
	case "Directory":
		return writeDirectory(req, parent, name)
	case "Symlink":
		return writeSymlink(req, parent, name)
	default:
		return writeFile(req, parent, name)
	}
}

// remove deletes a target through the held parent, so it cannot be redirected the
// way an unlink on a path string can.
func remove(req provider.Request) error {
	parent, name, err := openParent(req.Target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer parent.Close()

	// Never recursive, or it would remove things nobody declared.
	flags := 0
	if req.Ref.Type == "Directory" {
		flags = unix.AT_REMOVEDIR
	}
	if err := unix.Unlinkat(int(parent.Fd()), name, flags); err != nil {
		if err == unix.ENOENT {
			return nil
		}
		return &os.PathError{Op: "unlinkat", Path: req.Target, Err: err}
	}
	return parent.Sync()
}

// openParent opens the directory holding a target, without following a link.
func openParent(target string) (*os.File, string, error) {
	dir, name := filepath.Split(filepath.Clean(target))
	if name == "" {
		return nil, "", fmt.Errorf("%s has no final path component", target)
	}
	parent, err := os.OpenFile(filepath.Clean(dir),
		os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, "", err
	}
	return parent, name, nil
}

// writeFile stages content beside the target and renames it into place.
//
// Same directory, because a rename across filesystems is not atomic. Ownership and
// mode go on the descriptor first, so the file is never briefly readable by the
// wrong user.
func writeFile(req provider.Request, parent *os.File, name string) error {
	content, kind, err := desiredContent(req)
	if err != nil {
		return err
	}
	if kind == contentSecret {
		return fmt.Errorf("secret references are not implemented, so %s cannot be written", req.Ref)
	}
	// Metadata only, so keep what is there. This is how ownership on a file a
	// package created is fixed without taking over its content.
	if kind == contentNone {
		existing, err := os.ReadFile(req.Target)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		content = existing
	}

	temp, tempName, err := createTemp(parent)
	if err != nil {
		return err
	}
	// A failure from here leaves no trace, since the staged file goes and the live one was
	// never touched.
	defer func() {
		temp.Close()
		if tempName != "" {
			_ = unix.Unlinkat(int(parent.Fd()), tempName, 0)
		}
	}()

	if err := applyOwnership(req, temp); err != nil {
		return err
	}
	if err := applyMode(req, temp, 0o644); err != nil {
		return err
	}
	if _, err := temp.Write(content); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}

	if err := unix.Renameat(int(parent.Fd()), tempName, int(parent.Fd()), name); err != nil {
		return &os.PathError{Op: "renameat", Path: req.Target, Err: err}
	}
	tempName = ""

	// Sync the directory too, or a crash leaves the name on the old inode.
	return parent.Sync()
}

func writeDirectory(req provider.Request, parent *os.File, name string) error {
	// Restrictive first, widened after. The other order leaves a window where the
	// directory is world readable, and whatever got in stays in.
	err := unix.Mkdirat(int(parent.Fd()), name, 0o700)
	if err != nil && err != unix.EEXIST {
		return &os.PathError{Op: "mkdirat", Path: req.Target, Err: err}
	}

	fd, err := unix.Openat(int(parent.Fd()), name,
		os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return &os.PathError{Op: "openat", Path: req.Target, Err: err}
	}
	handle := os.NewFile(uintptr(fd), req.Target)
	defer handle.Close()

	if err := applyOwnership(req, handle); err != nil {
		return err
	}
	return applyMode(req, handle, 0o755)
}

// writeSymlink stages the link and renames it, because symlinkat fails on an
// existing name and removing the old one first leaves a gap.
func writeSymlink(req provider.Request, parent *os.File, name string) error {
	target, ok := req.Field("target")
	if !ok {
		return fmt.Errorf("%s has no desired.target", req.Ref)
	}

	staged := tempName()
	if err := unix.Symlinkat(target, int(parent.Fd()), staged); err != nil {
		return &os.PathError{Op: "symlinkat", Path: req.Target, Err: err}
	}
	if err := unix.Renameat(int(parent.Fd()), staged, int(parent.Fd()), name); err != nil {
		_ = unix.Unlinkat(int(parent.Fd()), staged, 0)
		return &os.PathError{Op: "renameat", Path: req.Target, Err: err}
	}
	return parent.Sync()
}

// createTemp makes a staging file in the held directory. O_EXCL means the name
// cannot be a file or link planted in advance.
func createTemp(parent *os.File) (*os.File, string, error) {
	for attempt := 0; attempt < 8; attempt++ {
		name := tempName()
		fd, err := unix.Openat(int(parent.Fd()), name,
			unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_NOFOLLOW, 0o600)
		if err == unix.EEXIST {
			// Retry with a new name, which is the point of O_EXCL.
			continue
		}
		if err != nil {
			return nil, "", &os.PathError{Op: "openat", Path: name, Err: err}
		}
		return os.NewFile(uintptr(fd), filepath.Join(parent.Name(), name)), name, nil
	}
	return nil, "", fmt.Errorf("could not create a staging file in %s", parent.Name())
}

// tempName is unpredictable, so it cannot be guessed and pre-created.
func tempName() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Only if the entropy source fails, at which point this is the lesser
		// problem.
		return ".datum.tmp.fallback"
	}
	return ".datum.tmp." + hex.EncodeToString(buf[:])
}

func applyOwnership(req provider.Request, handle *os.File) error {
	owner, hasOwner := req.Field("owner")
	group, hasGroup := req.Field("group")
	if !hasOwner && !hasGroup {
		return nil
	}

	uid, gid := -1, -1
	if hasOwner {
		id, err := lookupOwner(owner)
		if err != nil {
			return err
		}
		uid = id
	}
	if hasGroup {
		id, err := lookupGroup(group)
		if err != nil {
			return err
		}
		gid = id
	}
	// On the descriptor, so the file that changes is the one inspected.
	return handle.Chown(uid, gid)
}

// applyMode sets the declared bits, or a conventional default.
func applyMode(req provider.Request, handle *os.File, fallback fs.FileMode) error {
	text, ok := req.Field("mode")
	if !ok {
		return handle.Chmod(fallback)
	}
	mode, err := parseMode(text)
	if err != nil {
		return err
	}
	return handle.Chmod(mode)
}
