// SPDX-License-Identifier: Apache-2.0

package apt

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/run"
	"github.com/dsgnr/datum/internal/state"
)

// A package source is two files, the deb822 stanza apt reads and the key packages from
// it must be signed by. Both are written here so that neither can be left behind by the
// other.
const (
	sourcesDir = "etc/apt/sources.list.d"
	keyringDir = "etc/apt/keyrings"
)

// observeRepository reports the source's stanza and a digest of the key it trusts.
func (p *Provider) observeRepository(req provider.Request) (provider.Observation, error) {
	if err := validID(req.Target); err != nil {
		return provider.Observation{}, err
	}
	if err := p.rejectUnsupported(req); err != nil {
		return provider.Observation{}, err
	}

	desired, err := p.desiredRepository(req)
	if err != nil {
		return provider.Observation{}, err
	}
	observation := provider.Observation{Desired: desired}

	stanza, err := os.ReadFile(p.sourcesPath(req.Target))
	if os.IsNotExist(err) {
		return observation, nil
	}
	if err != nil {
		return provider.Observation{}, err
	}

	fields := parseStanza(string(stanza))
	observation.Exists = true
	observation.Fields = map[string]document.Value{
		"url":     document.Scalar(firstWord(fields["uris"])),
		"enabled": document.Scalar(boolFromYesNo(fields["enabled"])),
	}
	if suite := fields["suites"]; suite != "" {
		observation.Fields["suite"] = document.Scalar(firstWord(suite))
	}
	if components := fields["components"]; components != "" {
		observation.Fields["components"] = componentList(components)
	}

	// The key is compared as a digest because a rotated key is the change least
	// likely to be noticed by hand and the one that matters most.
	key, err := os.ReadFile(p.keyringPath(req.Target))
	switch {
	case err == nil:
		observation.Fields["signingKey"] = document.Scalar(provider.Digest(key))
	case os.IsNotExist(err):
		// A signed source whose key has gone is drift, not an absent source.
		observation.Fields["signingKey"] = document.Scalar("")
	default:
		return provider.Observation{}, err
	}
	return observation, nil
}

// desiredRepository restates the declared fields as they can be compared with a
// host. The signing key is a repository path on one side and bytes on the other, so
// both become digests.
func (p *Provider) desiredRepository(req provider.Request) (document.Value, error) {
	out := map[string]document.Value{}
	for name, value := range req.Desired.Map {
		if name == "signingKey" {
			continue
		}
		out[name] = value
	}
	// Defaulted here rather than left absent, so a host that has switched a source off
	// reads as drift against a manifest that never mentioned enabled.
	if _, ok := out["enabled"]; !ok && req.Present() {
		out["enabled"] = document.Scalar("true")
	}

	if rel, ok := req.Field("signingKey"); ok {
		key, err := req.ReadSource(rel)
		if err != nil {
			return document.Value{}, fmt.Errorf("apt: reading signingKey: %w", err)
		}
		out["signingKey"] = document.Scalar(provider.Digest(key))
	}
	return document.Value{Kind: document.KindMap, Map: out}, nil
}

// applyRepository writes or removes the source.
func (p *Provider) applyRepository(req provider.Request, action state.Action) error {
	if err := validID(req.Target); err != nil {
		return err
	}
	if err := p.rejectUnsupported(req); err != nil {
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
		return fmt.Errorf("apt: unexpected action %s", action)
	}
}

func (p *Provider) writeRepository(req provider.Request) error {
	url, ok := req.Field("url")
	if !ok || url == "" {
		return fmt.Errorf("apt: %s has no desired.url", req.Ref)
	}

	keyring := ""
	if rel, ok := req.Field("signingKey"); ok {
		key, err := req.ReadSource(rel)
		if err != nil {
			return fmt.Errorf("apt: reading signingKey: %w", err)
		}
		keyring = p.keyringPath(req.Target)
		if err := writeFile(keyring, key, 0o644); err != nil {
			return err
		}
	}

	stanza := buildStanza(req, url, keyring)
	return writeFile(p.sourcesPath(req.Target), []byte(stanza), 0o644)
}

// removeRepository takes the source out and leaves installed packages alone, which
// is what declaring a source absent means.
func (p *Provider) removeRepository(req provider.Request) error {
	for _, path := range []string{p.sourcesPath(req.Target), p.keyringPath(req.Target)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// rejectUnsupported fails instead of dropping a field this provider cannot express.
// Priority on an apt source means a pin in apt_preferences, which is a separate
// mechanism with its own matching rules, and guessing at one would produce a pin that
// looks applied and does nothing.
func (p *Provider) rejectUnsupported(req provider.Request) error {
	if value, ok := req.Field("priority"); ok && value != "" {
		return fmt.Errorf("apt: %s sets desired.priority, which this provider does not express", req.Ref)
	}
	return nil
}

func buildStanza(req provider.Request, url, keyring string) string {
	var b strings.Builder
	b.WriteString("Types: deb\n")
	b.WriteString("URIs: " + url + "\n")

	// apt needs a suite, and a source pointing at a flat directory says so with a
	// trailing slash. Defaulting keeps a flat repository declarable without a field
	// whose value would only ever be "/".
	suite := req.FieldOr("suite", "")
	if suite == "" && strings.HasSuffix(url, "/") {
		suite = "/"
	}
	if suite != "" {
		b.WriteString("Suites: " + suite + "\n")
	}
	if components := componentsOf(req); len(components) > 0 {
		b.WriteString("Components: " + strings.Join(components, " ") + "\n")
	}

	if keyring != "" {
		b.WriteString("Signed-By: " + keyring + "\n")
	} else {
		// No key means the source was declared unsigned, which the schema only
		// allows when it was said out loud.
		b.WriteString("Trusted: yes\n")
	}

	b.WriteString("Enabled: " + yesNo(req.FieldOr("enabled", "true")) + "\n")
	return b.String()
}

func componentsOf(req provider.Request) []string {
	value, ok := req.Desired.Lookup("components")
	if !ok || value.Kind != document.KindList {
		return nil
	}
	var out []string
	for _, item := range value.List {
		if item.Kind == document.KindScalar && item.Scalar != "" {
			out = append(out, item.Scalar)
		}
	}
	return out
}

// parseStanza reads deb822 into lowercased keys. Continuation lines are joined,
// because a multi-line URIs field means the same thing as a single-line one.
func parseStanza(text string) map[string]string {
	fields := map[string]string{}
	key := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if key != "" {
				fields[key] += " " + strings.TrimSpace(line)
			}
			continue
		}
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(name))
		fields[key] = strings.TrimSpace(value)
	}
	return fields
}

func componentList(value string) document.Value {
	parts := strings.Fields(value)
	sort.Strings(parts)
	items := make([]document.Value, 0, len(parts))
	for _, part := range parts {
		items = append(items, document.Scalar(part))
	}
	return document.Value{Kind: document.KindList, List: items}
}

func firstWord(value string) string {
	if fields := strings.Fields(value); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

// boolFromYesNo reads deb822's yes and no. An absent Enabled field means enabled,
// which is apt's own default.
func boolFromYesNo(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "no", "false":
		return "false"
	default:
		return "true"
	}
}

func yesNo(value string) string {
	if value == "false" {
		return "no"
	}
	return "yes"
}

func (p *Provider) sourcesPath(id string) string {
	return filepath.Join(p.root, sourcesDir, id+".sources")
}

func (p *Provider) keyringPath(id string) string {
	return filepath.Join(p.root, keyringDir, id+".asc")
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Written through a temporary file and renamed, so a reader never sees a
	// half-written source list.
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

// An apt source file name ends up in a path, so it is held to the same rules as a
// package name minus the version separators.
func validID(id string) error {
	if err := run.Word("repository id", id, "-._"); err != nil {
		return fmt.Errorf("apt: %w", err)
	}
	return nil
}
