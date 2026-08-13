// SPDX-License-Identifier: Apache-2.0

// Package procsys implements Sysctl through /proc/sys and /etc/sysctl.d.
//
// A kernel parameter exists in two places. The running kernel holds the current value,
// and a file under /etc/sysctl.d sets the value applied after a reboot. Both are
// observed, since either can differ on its own.
package procsys

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/state"
)

// Prefix numbers the provider's files so they load after a distribution's own.
const Prefix = "70-datum-"

// Provider satisfies Sysctl. The two roots are fields so the tests can use a
// directory instead of the kernel.
type Provider struct {
	procRoot string
	confDir  string
}

func New() *Provider {
	return &Provider{procRoot: "/proc/sys", confDir: "/etc/sysctl.d"}
}

// NewWith points the provider at other directories, for tests.
func NewWith(procRoot, confDir string) *Provider {
	return &Provider{procRoot: procRoot, confDir: confDir}
}

func (p *Provider) Name() string    { return "proc-sys" }
func (p *Provider) Types() []string { return []string{"Sysctl"} }

// Observe reports the running value and whether the provider's file agrees with it.
//
// Existence is whether Datum manages the parameter, meaning whether its file is there,
// not whether the kernel knows the key. A parameter the kernel does not have cannot be
// managed at all, and that is reported when the value is read.
func (p *Provider) Observe(ctx context.Context, req provider.Request) (provider.Observation, error) {
	key := req.Target
	if err := validKey(key); err != nil {
		return provider.Observation{}, err
	}

	running, readable, err := p.runningValue(key)
	if err != nil {
		return provider.Observation{}, err
	}

	stored, managed, err := p.storedValue(key)
	if err != nil {
		return provider.Observation{}, err
	}

	// persisted is measured against what was asked for, not against the running value.
	// Otherwise changing the kernel by hand reports the file as wrong when the file is the
	// only part still right.
	want, _ := req.Field("value")

	observation := provider.Observation{
		Exists: managed,
		Fields: map[string]document.Value{
			"value":     document.Scalar(running),
			"persisted": document.Scalar(boolText(managed && stored == want)),
		},
	}
	if !readable {
		// Nothing to compare against, and applying will say why.
		observation.Found = "no such kernel parameter"
		observation.Unobservable = append(observation.Unobservable, "value")
	}

	// persisted is not a field any document declares, so the provider supplies both
	// sides of the comparison. Without this, a value correct in the running kernel
	// and missing from the file would read as converged.
	if req.Present() {
		observation.Desired = document.Value{
			Kind: document.KindMap,
			Map: map[string]document.Value{
				"value":     document.Scalar(want),
				"persisted": document.Scalar("true"),
			},
		}
	}
	return observation, nil
}

// Apply sets the running value and writes the file that survives a reboot, or stops
// managing the parameter.
func (p *Provider) Apply(ctx context.Context, req provider.Request, action state.Action) error {
	key := req.Target
	if err := validKey(key); err != nil {
		return err
	}

	switch action {
	case state.Create, state.Update:
		value, ok := req.Field("value")
		if !ok {
			return fmt.Errorf("procsys: %s has no desired.value", req.Ref)
		}
		if err := validValue(value); err != nil {
			return err
		}
		if err := p.write(key, value); err != nil {
			return err
		}
		return p.persist(key, value)

	case state.Remove:
		// The running value is left as it is. There is no way to restore a kernel
		// default at runtime, because the default is not recorded anywhere once the
		// value has been changed.
		return p.unpersist(key)

	case state.None, state.Skip:
		return nil
	default:
		return fmt.Errorf("procsys: unexpected action %s", action)
	}
}

// runningValue reads the parameter from the kernel. A key the kernel does not have is
// reported unreadable instead of as an error, so a plan can say so.
func (p *Provider) runningValue(key string) (string, bool, error) {
	body, err := os.ReadFile(p.procPath(key))
	switch {
	case err == nil:
		return normalise(string(body)), true, nil
	case os.IsNotExist(err):
		return "", false, nil
	case os.IsPermission(err):
		// A container with /proc/sys mounted read only is the common case, and
		// saying that is better than a bare permission error.
		return "", false, fmt.Errorf("procsys: cannot read %s, which usually means /proc/sys is not available here: %w",
			key, err)
	default:
		return "", false, err
	}
}

// storedValue reads the value from the provider's own file.
func (p *Provider) storedValue(key string) (string, bool, error) {
	body, err := os.ReadFile(p.confPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}

	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		return normalise(value), true, nil
	}
	// The file is there and says nothing about this key, which is the same as not
	// being managed.
	return "", false, nil
}

// write sets the running value. procfs takes a plain write and cannot be replaced by
// a rename, so this is not the atomic write a regular file gets.
func (p *Provider) write(key, value string) error {
	path := p.procPath(key)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("procsys: this kernel has no parameter %s", key)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("procsys: cannot set %s, which usually means /proc/sys is read only here", key)
		}
		return err
	}
	defer file.Close()

	if _, err := file.WriteString(value + "\n"); err != nil {
		// The kernel rejects a value it will not accept at write time, so this is
		// where a bad value surfaces.
		return fmt.Errorf("procsys: the kernel refused %s for %s: %w", value, key, err)
	}
	return nil
}

// persist writes one file per parameter, so that a failure on one does not affect the
// others and two parameters changing in the same pass need not coordinate.
func (p *Provider) persist(key, value string) error {
	if err := os.MkdirAll(p.confDir, 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("# Managed by Datum. Edits are overwritten.\n%s = %s\n", key, value)
	return writeAtomic(p.confPath(key), []byte(body), 0o644)
}

func (p *Provider) unpersist(key string) error {
	err := os.Remove(p.confPath(key))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// procPath maps a parameter key onto its file. The kernel accepts either separator,
// and a key is the dotted form everywhere a document or sysctl output uses it.
func (p *Provider) procPath(key string) string {
	return filepath.Join(p.procRoot, filepath.FromSlash(strings.ReplaceAll(key, ".", "/")))
}

func (p *Provider) confPath(key string) string {
	return filepath.Join(p.confDir, Prefix+key+".conf")
}

// writeAtomic keeps a half-written file from being loaded at the next boot.
func writeAtomic(path string, body []byte, mode fs.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".datum-sysctl-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

// normalise collapses the whitespace the kernel uses between fields, so that a value
// of three numbers compares equal however many spaces or tabs separate them.
func normalise(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// validKey rejects anything that is not a parameter key.
//
// A key becomes a path, so this is what stops one escaping /proc/sys. Dots are the
// separator, which means an interface name containing a dot cannot be addressed in
// this form, and nothing in the model expresses one yet.
func validKey(key string) error {
	if key == "" {
		return fmt.Errorf("procsys: empty parameter key")
	}
	if strings.HasPrefix(key, ".") || strings.HasSuffix(key, ".") || strings.Contains(key, "..") {
		return fmt.Errorf("procsys: %q is not a parameter key", key)
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '-', r == '_':
		default:
			return fmt.Errorf("procsys: %q is not a parameter key", key)
		}
	}
	return nil
}

// validValue rejects a value that would turn one setting into several when written to
// a file that gets parsed again at boot.
func validValue(value string) error {
	if value == "" {
		return fmt.Errorf("procsys: empty value")
	}
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("procsys: a value cannot span lines")
	}
	return nil
}

// Detect reports whether kernel parameters can be managed here.
//
// A container with /proc/sys absent or mounted read only is the case where the resource
// cannot work at all, and skipping with that reason is better than failing every
// parameter on a permission error.
func Detect() bool {
	info, err := os.Stat("/proc/sys")
	if err != nil || !info.IsDir() {
		return false
	}
	// Probing one parameter every kernel has, because the directory being present
	// says nothing about whether it can be written.
	file, err := os.OpenFile("/proc/sys/kernel/hostname", os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	file.Close()
	return true
}
