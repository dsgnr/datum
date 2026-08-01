// SPDX-License-Identifier: Apache-2.0

package procsys

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/state"
)

const key = "net.ipv4.ip_forward"

// host is a pretend /proc/sys and /etc/sysctl.d, so these tests need neither a kernel
// nor root.
type host struct {
	proc string
	conf string
	p    *Provider
}

func newHost(t *testing.T) *host {
	t.Helper()
	root := t.TempDir()
	proc := filepath.Join(root, "proc")
	conf := filepath.Join(root, "conf")
	if err := os.MkdirAll(proc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(conf, 0o755); err != nil {
		t.Fatal(err)
	}
	return &host{proc: proc, conf: conf, p: NewWith(proc, conf)}
}

// kernel creates the file the running value lives in.
func (h *host) kernel(t *testing.T, name, value string) {
	t.Helper()
	path := filepath.Join(h.proc, filepath.FromSlash(strings.ReplaceAll(name, ".", "/")))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (h *host) running(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(h.proc, filepath.FromSlash(strings.ReplaceAll(name, ".", "/")))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(body))
}

func (h *host) confFile(t *testing.T, name string) (string, bool) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(h.conf, Prefix+name+".conf"))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(body), true
}

func request(name string, fields map[string]string) provider.Request {
	desired := document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}
	for k, v := range fields {
		desired.Map[k] = document.Scalar(v)
	}
	return provider.Request{
		Ref:     document.Reference{Type: "Sysctl", Name: name},
		Target:  name,
		Desired: desired,
	}
}

func present(value string) map[string]string {
	return map[string]string{"state": "present", "value": value}
}

func TestObserveUnmanagedParameter(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "0")

	got, err := h.p.Observe(context.Background(), request(key, present("1")))
	if err != nil {
		t.Fatal(err)
	}
	// Existence is whether Datum manages the parameter, and nothing has yet.
	if got.Exists {
		t.Error("a parameter with no file should not be reported as managed")
	}
	if v, _ := got.Value("value"); v.Scalar != "0" {
		t.Errorf("value = %q, want the running value", v.Scalar)
	}
	if v, _ := got.Value("persisted"); v.Scalar != "false" {
		t.Errorf("persisted = %q, want false", v.Scalar)
	}
}

func TestApplyWritesBothPlaces(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "0")

	err := h.p.Apply(context.Background(), request(key, present("1")), state.Create)
	if err != nil {
		t.Fatal(err)
	}

	if got := h.running(t, key); got != "1" {
		t.Errorf("running value = %q, want 1", got)
	}
	body, ok := h.confFile(t, key)
	if !ok {
		t.Fatal("the parameter should have been persisted")
	}
	if !strings.Contains(body, key+" = 1") {
		t.Errorf("conf file = %q", body)
	}
}

// A value set in the running kernel and never written to a file is the drift that is
// hardest to notice, so the provider supplies both sides of that comparison.
func TestRunningValueCorrectButNotPersistedIsDrift(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "1")

	got, err := h.p.Observe(context.Background(), request(key, present("1")))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("persisted"); v.Scalar != "false" {
		t.Errorf("persisted = %q, want false", v.Scalar)
	}
	// The provider's own view of desired state is what makes persisted comparable,
	// because no document declares it.
	want, ok := got.Desired.Lookup("persisted")
	if !ok || want.Scalar != "true" {
		t.Errorf("desired persisted = %+v, want true", want)
	}
}

// Persisted and correct in both places is the converged case.
func TestConvergedParameterReportsBothAsMatching(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "0")
	if err := h.p.Apply(context.Background(), request(key, present("1")), state.Create); err != nil {
		t.Fatal(err)
	}

	got, err := h.p.Observe(context.Background(), request(key, present("1")))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exists {
		t.Error("the parameter should be managed now")
	}
	if v, _ := got.Value("value"); v.Scalar != "1" {
		t.Errorf("value = %q", v.Scalar)
	}
	if v, _ := got.Value("persisted"); v.Scalar != "true" {
		t.Errorf("persisted = %q, want true", v.Scalar)
	}
}

// Somebody changing the running kernel by hand leaves the file correct, so only the
// running value is drift. Measuring persisted against the running value instead would
// report the one part that is still right as wrong.
func TestKernelChangedByHandLeavesTheFilePersisted(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "0")
	if err := h.p.Apply(context.Background(), request(key, present("1")), state.Create); err != nil {
		t.Fatal(err)
	}
	// Somebody ran sysctl -w and put it back.
	h.kernel(t, key, "0")

	got, err := h.p.Observe(context.Background(), request(key, present("1")))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("persisted"); v.Scalar != "true" {
		t.Errorf("persisted = %q, want true because the file still says 1", v.Scalar)
	}
	if v, _ := got.Value("value"); v.Scalar != "0" {
		t.Errorf("value = %q, want the running value", v.Scalar)
	}
}

// A file holding a value nobody asked for is not persisted correctly, which is what a
// hand-edited file looks like.
func TestFileWithTheWrongValueIsNotPersisted(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "1")
	if err := os.WriteFile(filepath.Join(h.conf, Prefix+key+".conf"),
		[]byte(key+" = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := h.p.Observe(context.Background(), request(key, present("1")))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("persisted"); v.Scalar != "false" {
		t.Errorf("persisted = %q, want false", v.Scalar)
	}
}

// Removal takes the file away and leaves the running value, because there is no way to
// restore a kernel default once the value has been changed.
func TestRemoveLeavesTheRunningValue(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "0")
	if err := h.p.Apply(context.Background(), request(key, present("1")), state.Create); err != nil {
		t.Fatal(err)
	}

	err := h.p.Apply(context.Background(),
		request(key, map[string]string{"state": "absent"}), state.Remove)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := h.confFile(t, key); ok {
		t.Error("the file should have been removed")
	}
	if got := h.running(t, key); got != "1" {
		t.Errorf("running value = %q, want the value left alone", got)
	}
}

func TestRemoveOfSomethingAlreadyGoneIsNotAnError(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "0")

	err := h.p.Apply(context.Background(),
		request(key, map[string]string{"state": "absent"}), state.Remove)
	if err != nil {
		t.Errorf("err = %v", err)
	}
}

// A parameter this kernel does not have cannot be managed, and saying so beats
// reporting a value of nothing.
func TestUnknownParameterIsReported(t *testing.T) {
	h := newHost(t)

	got, err := h.p.Observe(context.Background(), request("net.ipv4.no_such_thing", present("1")))
	if err != nil {
		t.Fatal(err)
	}
	if got.CanObserve("value") {
		t.Error("the value of a parameter that does not exist is not observable")
	}
	if !strings.Contains(got.Found, "no such kernel parameter") {
		t.Errorf("Found = %q", got.Found)
	}
}

func TestApplyToAnUnknownParameterFails(t *testing.T) {
	h := newHost(t)

	err := h.p.Apply(context.Background(), request("net.ipv4.no_such_thing", present("1")), state.Create)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "no parameter") {
		t.Errorf("err = %v", err)
	}
}

// Several parameters hold whitespace-separated numbers, so the comparison has to be
// insensitive to how many spaces the kernel used.
func TestWhitespaceInValuesIsNormalised(t *testing.T) {
	h := newHost(t)
	name := "net.ipv4.tcp_rmem"
	h.kernel(t, name, "4096\t131072   6291456")

	got, err := h.p.Observe(context.Background(), request(name, present("4096 131072 6291456")))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("value"); v.Scalar != "4096 131072 6291456" {
		t.Errorf("value = %q", v.Scalar)
	}
}

// One file per parameter keeps resources independent, so two of them in one pass do not
// contend over a shared file.
func TestEachParameterGetsItsOwnFile(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "0")
	h.kernel(t, "vm.swappiness", "60")

	for name, value := range map[string]string{key: "1", "vm.swappiness": "10"} {
		if err := h.p.Apply(context.Background(), request(name, present(value)), state.Create); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(h.conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d files, want one per parameter", len(entries))
	}
	for _, name := range []string{key, "vm.swappiness"} {
		if _, ok := h.confFile(t, name); !ok {
			t.Errorf("%s has no file", name)
		}
	}
}

// The file is loaded again at boot, so a half-written one would be a bad parameter
// rather than a missing one.
func TestNoTemporaryFilesAreLeftBehind(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "0")
	if err := h.p.Apply(context.Background(), request(key, present("1")), state.Create); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(h.conf)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			t.Errorf("leftover temporary file %s", entry.Name())
		}
	}
}

// A key becomes a path, so this is what stops one escaping /proc/sys.
func TestRejectsKeysThatAreNotParameterKeys(t *testing.T) {
	h := newHost(t)

	for _, name := range []string{
		"",
		"../../../etc/passwd",
		"net..ipv4",
		".net.ipv4",
		"net.ipv4.",
		"net/ipv4/ip_forward",
		"net.ipv4.ip_forward;reboot",
	} {
		if _, err := h.p.Observe(context.Background(), request(name, present("1"))); err == nil {
			t.Errorf("Observe accepted %q", name)
		}
		if err := h.p.Apply(context.Background(), request(name, present("1")), state.Create); err == nil {
			t.Errorf("Apply accepted %q", name)
		}
	}
}

// A value with a newline in it would become two settings in a file that gets parsed
// again at boot.
func TestRejectsValuesThatWouldSplitIntoSeveralSettings(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "0")

	for _, value := range []string{"1\nkernel.panic = 0", "", "1\r\nx = y"} {
		err := h.p.Apply(context.Background(), request(key, present(value)), state.Create)
		if err == nil {
			t.Errorf("Apply accepted value %q", value)
		}
	}
	if body, ok := h.confFile(t, key); ok {
		t.Errorf("nothing should have been written, got %q", body)
	}
}

func TestAcceptsRealParameterKeys(t *testing.T) {
	for _, name := range []string{
		"net.ipv4.ip_forward",
		"vm.swappiness",
		"kernel.hostname",
		"net.ipv4.conf.all.rp_filter",
		"net.bridge.bridge-nf-call-iptables",
	} {
		if err := validKey(name); err != nil {
			t.Errorf("validKey(%q) = %v", name, err)
		}
	}
}

func TestNoActionChangesNothing(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "0")

	for _, action := range []state.Action{state.None, state.Skip} {
		if err := h.p.Apply(context.Background(), request(key, present("1")), action); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.running(t, key); got != "0" {
		t.Errorf("running value = %q, want it untouched", got)
	}
	if _, ok := h.confFile(t, key); ok {
		t.Error("nothing should have been persisted")
	}
}

// Another file setting the same key is not this provider's file, so the parameter is
// not managed by Datum.
func TestAnotherFileDoesNotCountAsManaged(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "1")
	other := filepath.Join(h.conf, "10-distribution.conf")
	if err := os.WriteFile(other, []byte(key+" = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := h.p.Observe(context.Background(), request(key, present("1")))
	if err != nil {
		t.Fatal(err)
	}
	if got.Exists {
		t.Error("somebody else's file does not make the parameter managed by Datum")
	}
}

// A file the provider owns that says nothing about the key is the same as not managing
// it, which is what a truncated or hand-edited file looks like.
func TestOwnFileWithoutTheKeyIsNotManaged(t *testing.T) {
	h := newHost(t)
	h.kernel(t, key, "1")
	if err := os.WriteFile(filepath.Join(h.conf, Prefix+key+".conf"),
		[]byte("# emptied by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := h.p.Observe(context.Background(), request(key, present("1")))
	if err != nil {
		t.Fatal(err)
	}
	if got.Exists {
		t.Error("a file that sets nothing does not manage the parameter")
	}
}

func TestTypesAndName(t *testing.T) {
	p := New()
	if p.Name() != "proc-sys" {
		t.Errorf("Name = %q", p.Name())
	}
	if types := p.Types(); len(types) != 1 || types[0] != "Sysctl" {
		t.Errorf("Types = %v", types)
	}
}

func TestSatisfiesTheProviderInterface(t *testing.T) {
	var _ provider.Provider = New()
}
