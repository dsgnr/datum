// SPDX-License-Identifier: Apache-2.0

// Package anchor refuses desired state that targets Datum's own controls.
//
// The protected set covers the signer list, the configuration naming the source, the
// fetch credential, the state directory holding the accepted revision, and the agent
// itself. A File resource able to write the signer list would allow one commit to
// install a key that every later commit verifies against.
//
// The set is enumerated explicitly. It does not extend to every path that grants root,
// which would cover most of /etc.
package anchor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsgnr/datum/internal/config"
	"github.com/dsgnr/datum/internal/document"
)

// Set is what a host protects.
type Set struct {
	// Files are exact paths. A resource whose target resolves to one is refused.
	Files []string
	// Dirs are protected prefixes. Anything at or beneath one is refused, and so is
	// any ancestor of one, because a resource owning /etc owns everything under it.
	Dirs []string
	// Package is the agent's own package name, which `state: absent` would remove.
	Package string
	// Unit is the agent's own service unit, which `state: stopped` would silence.
	Unit string
}

// Defaults are the paths in the documentation, for a host whose configuration has not
// moved anything.
const (
	DefaultConfig = config.Path
	// The name the agent's package and unit take. Both are the same word in practice, and
	// a fleet that renames either loses this protection for it, which is why the names are
	// part of the set rather than compiled in.
	DefaultName = "datum"
)

// binaryPaths are the conventional install locations. A package uses /usr/bin and a
// hand-copied build usually goes to /usr/local/bin, and protecting one leaves the other
// replaceable by a commit.
var binaryPaths = []string{"/usr/bin/datum", "/usr/local/bin/datum"}

// For builds the protected set from an agent's configuration.
//
// Derived from configuration, not hard-coded. A host that keeps its state somewhere
// else still needs that directory protected, and protecting the default while the real
// one is writable is worse than protecting nothing.
func For(cfg config.Config) Set {
	set := Set{
		Files:   append([]string{DefaultConfig}, binaryPaths...),
		Dirs:    []string{filepath.Dir(DefaultConfig)},
		Package: DefaultName,
		Unit:    DefaultName + ".service",
	}
	// Wherever this process actually runs from, which covers an install location neither
	// convention above predicts. A resource that can overwrite the running binary can
	// replace the thing enforcing every other refusal.
	if self, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(self); err == nil {
			self = resolved
		}
		set.Files = append(set.Files, self)
	}
	if cfg.Trust.Signers != "" {
		set.Files = append(set.Files, cfg.Trust.Signers)
	}
	if cfg.State != "" {
		set.Dirs = append(set.Dirs, cfg.State)
	}
	if cfg.Source.Credential != "" {
		set.Files = append(set.Files, cfg.Source.Credential)
	}
	if cfg.Secrets.Path != "" {
		set.Dirs = append(set.Dirs, cfg.Secrets.Path)
	}
	return set
}

// Default is the set for a host running with stock paths, which is what a command with no
// configuration to read has to assume.
func Default() Set {
	cfg := config.Default()
	return For(cfg)
}

// pathTypes are the resource types whose target identity is a filesystem path.
var pathTypes = map[string]bool{"File": true, "Directory": true, "Symlink": true}

// Check refuses a resource that targets one of the protected things.
//
// The error names the file and the control it protects, since "refused" alone reads as
// a manifest error rather than a boundary.
func (s Set) Check(ref document.Reference, target string, desired document.Value) error {
	switch {
	case pathTypes[ref.Type]:
		if err := s.checkPath(ref, target); err != nil {
			return err
		}
		// A Symlink is refused by its own path above, and also by where it points, since a
		// link at an innocent path whose target is the signer list would otherwise replace
		// that file's content through the link.
		if ref.Type == "Symlink" {
			if to, ok := desired.Lookup("target"); ok && to.Kind == document.KindScalar {
				if err := s.checkPath(ref, to.Scalar); err != nil {
					return err
				}
			}
		}
		return nil

	case ref.Type == "Package":
		if s.Package != "" && target == s.Package {
			return fmt.Errorf("%s targets the agent's own package, which desired state cannot manage, because removing it would stop the host being managed at all", ref)
		}
	case ref.Type == "Service":
		if s.Unit != "" && (target == s.Unit || target == strings.TrimSuffix(s.Unit, ".service")) {
			return fmt.Errorf("%s targets the agent's own service, which desired state cannot manage, because stopping it would stop the host reconciling", ref)
		}
	}
	return nil
}

// checkPath refuses a path that is, contains, or sits inside protected state.
func (s Set) checkPath(ref document.Reference, target string) error {
	if target == "" || !filepath.IsAbs(target) {
		// Not this check's business. A path that is not absolute is refused by field
		// validation, and reporting it here would give two errors for one mistake.
		return nil
	}
	// Lexical cleaning handles the /etc/datum/../datum/agent.yaml route. Links are not
	// resolved here, since this check runs against a manifest with no host in the picture
	// and a resource is refused for what it declares. CheckIdentity catches the routes
	// that only exist on a host, such as a hard link into a protected file.
	clean := filepath.Clean(target)

	for _, file := range s.Files {
		protected := filepath.Clean(file)
		switch {
		case clean == protected:
			return fmt.Errorf("%s targets %s, which %s", ref, protected, protects(protected))
		case within(clean, protected):
			// The target is an ancestor of a protected file, so owning it owns the file.
			return fmt.Errorf("%s targets %s, which contains %s, and %s",
				ref, clean, protected, protects(protected))
		}
	}

	for _, dir := range s.Dirs {
		protected := filepath.Clean(dir)
		switch {
		case clean == protected || within(protected, clean):
			return fmt.Errorf("%s targets %s, which is inside %s, and %s",
				ref, clean, protected, protects(protected))
		case within(clean, protected):
			return fmt.Errorf("%s targets %s, which contains %s, and %s",
				ref, clean, protected, protects(protected))
		}
	}
	return nil
}

// within reports whether child is at or beneath parent.
func within(parent, child string) bool {
	if parent == child {
		return true
	}
	if parent == string(filepath.Separator) {
		return strings.HasPrefix(child, parent)
	}
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}

// protects says what a path is defending, so the error explains the boundary.
func protects(path string) string {
	switch {
	case strings.HasSuffix(path, "agent.yaml"):
		return "holds this host's identity, its source and the trust settings themselves"
	case strings.Contains(path, "allowed-signers"), strings.Contains(path, "signers"):
		return "holds the keys that authorise desired state, so managing it would let a repository approve its own replacement"
	case strings.Contains(path, "credential"):
		return "holds the credential used to read the repository"
	case strings.Contains(path, "secret"):
		return "holds secret material resolved on this host"
	case strings.HasSuffix(path, "datum") && strings.Contains(path, "bin"):
		return "is the agent binary"
	default:
		return "holds the accepted revision, the pass lock and pass reports"
	}
}
