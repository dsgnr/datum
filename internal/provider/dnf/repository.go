// SPDX-License-Identifier: Apache-2.0

package dnf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/run"
	"github.com/dsgnr/datum/internal/state"
)

// dnf keeps a source in one ini file, with the key alongside it, not inside.
const (
	reposDir = "etc/yum.repos.d"
	keyDir   = "etc/pki/rpm-gpg"
)

// observeRepository reports the source's ini fields and a digest of its key.
func (p *Provider) observeRepository(req provider.Request) (provider.Observation, error) {
	if err := validID(req.Target); err != nil {
		return provider.Observation{}, err
	}
	if err := rejectUnsupported(req); err != nil {
		return provider.Observation{}, err
	}

	desired, err := desiredRepository(req)
	if err != nil {
		return provider.Observation{}, err
	}
	observation := provider.Observation{Desired: desired}

	text, err := os.ReadFile(p.repoPath(req.Target))
	if os.IsNotExist(err) {
		return observation, nil
	}
	if err != nil {
		return provider.Observation{}, err
	}

	fields := parseIni(string(text))
	observation.Exists = true
	observation.Fields = map[string]document.Value{
		"url":     document.Scalar(fields["baseurl"]),
		"enabled": document.Scalar(boolFromDigit(fields["enabled"], true)),
	}
	if priority := fields["priority"]; priority != "" {
		observation.Fields["priority"] = document.Scalar(priority)
	}

	key, err := os.ReadFile(p.keyPath(req.Target))
	switch {
	case err == nil:
		observation.Fields["signingKey"] = document.Scalar(provider.Digest(key))
	case os.IsNotExist(err):
		// A source declared signed whose key has gone is drift on that field, not a
		// missing source.
		observation.Fields["signingKey"] = document.Scalar("")
	default:
		return provider.Observation{}, err
	}
	return observation, nil
}

func desiredRepository(req provider.Request) (document.Value, error) {
	out := map[string]document.Value{}
	for name, value := range req.Desired.Map {
		if name == "signingKey" {
			continue
		}
		out[name] = value
	}
	if _, ok := out["enabled"]; !ok && req.Present() {
		out["enabled"] = document.Scalar("true")
	}

	if rel, ok := req.Field("signingKey"); ok {
		key, err := req.ReadSource(rel)
		if err != nil {
			return document.Value{}, fmt.Errorf("dnf: reading signingKey: %w", err)
		}
		out["signingKey"] = document.Scalar(provider.Digest(key))
	}
	return document.Value{Kind: document.KindMap, Map: out}, nil
}

func (p *Provider) applyRepository(req provider.Request, action state.Action) error {
	if err := validID(req.Target); err != nil {
		return err
	}
	if err := rejectUnsupported(req); err != nil {
		return err
	}

	switch action {
	case state.Create, state.Update:
		return p.writeRepository(req)
	case state.Remove:
		return p.removeRepository(req)
	case state.None, state.Skip:
		return nil
	default:
		return fmt.Errorf("dnf: unexpected action %s", action)
	}
}

func (p *Provider) writeRepository(req provider.Request) error {
	url, ok := req.Field("url")
	if !ok || url == "" {
		return fmt.Errorf("dnf: %s has no desired.url", req.Ref)
	}

	keyPath := ""
	if rel, ok := req.Field("signingKey"); ok {
		key, err := req.ReadSource(rel)
		if err != nil {
			return fmt.Errorf("dnf: reading signingKey: %w", err)
		}
		keyPath = p.keyPath(req.Target)
		if err := writeFile(keyPath, key, 0o644); err != nil {
			return err
		}
	}
	return writeFile(p.repoPath(req.Target), []byte(buildRepo(req, url, keyPath)), 0o644)
}

func (p *Provider) removeRepository(req provider.Request) error {
	for _, path := range []string{p.repoPath(req.Target), p.keyPath(req.Target)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// rejectUnsupported refuses the two apt-shaped fields rather than dropping them. A dnf
// source has no suite or components, so accepting them would mean writing a source that
// ignores part of what was asked for.
func rejectUnsupported(req provider.Request) error {
	for _, name := range []string{"suite", "components"} {
		if value, ok := req.Desired.Lookup(name); ok && !emptyValue(value) {
			return fmt.Errorf("dnf: %s sets desired.%s, which an rpm source has no equivalent of", req.Ref, name)
		}
	}
	return nil
}

func emptyValue(value document.Value) bool {
	switch value.Kind {
	case document.KindScalar:
		return value.Scalar == ""
	case document.KindList:
		return len(value.List) == 0
	default:
		return false
	}
}

func buildRepo(req provider.Request, url, keyPath string) string {
	var b strings.Builder
	b.WriteString("[" + req.Target + "]\n")
	b.WriteString("name=" + req.Target + "\n")
	b.WriteString("baseurl=" + url + "\n")
	b.WriteString("enabled=" + digitFromBool(req.FieldOr("enabled", "true")) + "\n")

	if keyPath != "" {
		b.WriteString("gpgcheck=1\n")
		b.WriteString("gpgkey=file://" + keyPath + "\n")
	} else {
		// Reached only for a source the manifest declared unsigned, which the schema
		// requires to be said out loud.
		b.WriteString("gpgcheck=0\n")
	}

	if priority, ok := req.Field("priority"); ok && priority != "" {
		b.WriteString("priority=" + priority + "\n")
	}
	return b.String()
}

// parseIni reads the single section a repo file written here holds. Keys are
// lowercased, and the section header is skipped because the id is already known.
func parseIni(text string) map[string]string {
	fields := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		name, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		fields[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
	}
	return fields
}

// boolFromDigit reads dnf's 1 and 0. An absent value takes the given default,
// because dnf treats a missing enabled as enabled.
func boolFromDigit(value string, fallback bool) string {
	switch value {
	case "1", "true", "yes":
		return "true"
	case "0", "false", "no":
		return "false"
	default:
		if fallback {
			return "true"
		}
		return "false"
	}
}

func digitFromBool(value string) string {
	if value == "false" {
		return "0"
	}
	return "1"
}

func (p *Provider) repoPath(id string) string {
	return filepath.Join(p.root, reposDir, id+".repo")
}

func (p *Provider) keyPath(id string) string {
	return filepath.Join(p.root, keyDir, id+".asc")
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Renamed into place so dnf never reads a half-written source.
	temp := path + ".datum-tmp"
	if err := os.WriteFile(temp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}

// The id becomes both a filename and an ini section name, so it is held to the
// characters that are safe in each.
func validID(id string) error {
	if err := run.Word("repository id", id, "-._"); err != nil {
		return fmt.Errorf("dnf: %w", err)
	}
	return nil
}
