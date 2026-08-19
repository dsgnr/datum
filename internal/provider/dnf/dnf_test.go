// SPDX-License-Identifier: Apache-2.0

package dnf

import (
	"context"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/run/runtest"
	"github.com/dsgnr/datum/internal/state"
)

// The query format is the contract with rpm, so the tests pin the exact call.
func queryCall(name string) []string {
	return []string{"rpm", "--query", `--queryformat=%{VERSION}-%{RELEASE}\n`, "--", name}
}

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
	runner := runtest.New().Output("5.02-22.fc41\n", queryCall("sl")...)

	got, err := NewWith(runner).Observe(context.Background(), request("sl", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exists {
		t.Error("the package should be reported as installed")
	}
	if v, _ := got.Value("version"); v.Scalar != "5.02-22.fc41" {
		t.Errorf("version = %q", v.Scalar)
	}
}

// rpm says a package is not installed on stdout, where dpkg-query uses stderr.
// Assuming apt's behaviour here would turn every absent package into a failure.
func TestNotInstalledIsReportedOnStdout(t *testing.T) {
	runner := runtest.New().Reply(runtest.Reply{
		Code:   1,
		Stdout: "package sl is not installed\n",
	}, queryCall("sl")...)

	got, err := NewWith(runner).Observe(context.Background(), request("sl", nil))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got.Exists {
		t.Error("an absent package is not installed")
	}
}

func TestObserveOtherFailureIsAnError(t *testing.T) {
	runner := runtest.New().Fail(1, "error: rpmdb open failed\n", queryCall("sl")...)

	if _, err := NewWith(runner).Observe(context.Background(), request("sl", nil)); err == nil {
		t.Fatal("a broken rpmdb should not be read as absent")
	}
}

func TestInstallWithoutAVersion(t *testing.T) {
	runner := runtest.New()

	if err := NewWith(runner).Apply(context.Background(), request("sl", nil), state.Create); err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("dnf")
	if err != nil {
		t.Fatal(err)
	}
	if argv[1] != "install" || argv[len(argv)-1] != "sl" {
		t.Errorf("argv = %v", argv)
	}
	// Nothing needs comparing when no version was asked for.
	if runner.Ran(queryCall("sl")...) {
		t.Error("rpm should not have been queried")
	}
}

func TestInstallAMissingPackageAtAVersion(t *testing.T) {
	runner := runtest.New().Reply(runtest.Reply{
		Code:   1,
		Stdout: "package sl is not installed\n",
	}, queryCall("sl")...)

	err := NewWith(runner).Apply(context.Background(),
		request("sl", map[string]string{"version": "5.02-22.fc41"}), state.Create)
	if err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("dnf")
	if err != nil {
		t.Fatal(err)
	}
	if argv[1] != "install" || argv[len(argv)-1] != "sl-5.02-22.fc41" {
		t.Errorf("argv = %v", argv)
	}
}

// dnf install will not go backwards, so a pinned version lower than the installed one
// is a downgrade. Which it is comes from rpm's own comparison.
func TestPinningALowerVersionDowngrades(t *testing.T) {
	runner := runtest.New().
		Output("5.02-23.fc41\n", queryCall("sl")...).
		Output("-1\n", "rpm", "--eval", `%{lua: print(rpm.vercmp("5.02-22.fc41", "5.02-23.fc41"))}`)

	err := NewWith(runner).Apply(context.Background(),
		request("sl", map[string]string{"version": "5.02-22.fc41"}), state.Update)
	if err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("dnf")
	if err != nil {
		t.Fatal(err)
	}
	if argv[1] != "downgrade" {
		t.Errorf("argv = %v, want downgrade", argv)
	}
	if argv[len(argv)-1] != "sl-5.02-22.fc41" {
		t.Errorf("argv = %v", argv)
	}
}

func TestPinningAHigherVersionInstalls(t *testing.T) {
	runner := runtest.New().
		Output("5.02-22.fc41\n", queryCall("sl")...).
		Output("1\n", "rpm", "--eval", `%{lua: print(rpm.vercmp("5.02-23.fc41", "5.02-22.fc41"))}`)

	err := NewWith(runner).Apply(context.Background(),
		request("sl", map[string]string{"version": "5.02-23.fc41"}), state.Update)
	if err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("dnf")
	if err != nil {
		t.Fatal(err)
	}
	if argv[1] != "install" {
		t.Errorf("argv = %v, want install", argv)
	}
}

// A package already at the pinned version needs no transaction at all.
func TestPinningTheInstalledVersionRunsNothing(t *testing.T) {
	runner := runtest.New().
		Output("5.02-22.fc41\n", queryCall("sl")...).
		Output("0\n", "rpm", "--eval", `%{lua: print(rpm.vercmp("5.02-22.fc41", "5.02-22.fc41"))}`)

	err := NewWith(runner).Apply(context.Background(),
		request("sl", map[string]string{"version": "5.02-22.fc41"}), state.Update)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Args("dnf"); err == nil {
		t.Errorf("dnf should not have run, calls = %v", runner.Calls())
	}
}

func TestRemove(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(),
		request("sl", map[string]string{"state": "absent"}), state.Remove)
	if err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("dnf")
	if err != nil {
		t.Fatal(err)
	}
	if argv[1] != "remove" || argv[len(argv)-1] != "sl" {
		t.Errorf("argv = %v", argv)
	}
}

// A transaction that cannot be resolved has to fail rather than quietly doing part of
// what was asked.
func TestTransactionsAreStrict(t *testing.T) {
	runner := runtest.New()

	if err := NewWith(runner).Apply(context.Background(), request("sl", nil), state.Create); err != nil {
		t.Fatal(err)
	}
	argv, _ := runner.Args("dnf")
	line := strings.Join(argv, " ")
	for _, want := range []string{"--assumeyes", "--setopt=strict=1"} {
		if !strings.Contains(line, want) {
			t.Errorf("argv = %v, want %s", argv, want)
		}
	}
}

func TestApplyReportsDnfFailure(t *testing.T) {
	runner := runtest.New()
	runner.Default = runtest.Reply{Code: 1, Stderr: "No match for argument: sl\n"}

	err := NewWith(runner).Apply(context.Background(), request("sl", nil), state.Create)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "No match for argument") {
		t.Errorf("err = %v", err)
	}
}

// An ordering rpm could not produce has to be an error rather than a silent zero,
// which would look like a package already at the right version.
func TestUnreadableComparisonIsAnError(t *testing.T) {
	runner := runtest.New().
		Output("5.02-23.fc41\n", queryCall("sl")...).
		Output("what\n", "rpm", "--eval", `%{lua: print(rpm.vercmp("5.02-22.fc41", "5.02-23.fc41"))}`)

	err := NewWith(runner).Apply(context.Background(),
		request("sl", map[string]string{"version": "5.02-22.fc41"}), state.Update)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "ordering") {
		t.Errorf("err = %v", err)
	}
}

func TestNoActionRunsNothing(t *testing.T) {
	runner := runtest.New()

	for _, action := range []state.Action{state.None, state.Skip} {
		if err := NewWith(runner).Apply(context.Background(), request("sl", nil), action); err != nil {
			t.Fatal(err)
		}
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("nothing should have run, got %v", calls)
	}
}

// The version reaches rpm's Lua evaluator, so quotes and parentheses have to be
// refused as well as anything dnf would read as an option.
func TestRejectsVersionsThatCouldReachTheLuaEvaluator(t *testing.T) {
	runner := runtest.New()
	p := NewWith(runner)

	for _, version := range []string{
		`1.0") os.execute("touch /tmp/x`,
		`1.0'`,
		`1.0)`,
		"--option",
		"1.0 2.0",
	} {
		err := p.Apply(context.Background(),
			request("sl", map[string]string{"version": version}), state.Create)
		if err == nil {
			t.Errorf("Apply accepted version %q", version)
		}
	}
	for _, call := range runner.Calls() {
		if strings.Contains(call, "os.execute") {
			t.Fatalf("a rejected version reached rpm: %s", call)
		}
	}
}

func TestRejectsNamesThatAreNotPackageNames(t *testing.T) {
	runner := runtest.New()
	p := NewWith(runner)

	for _, name := range []string{"", "--assumeyes", "sl; rpm -e bash", "../../etc/passwd", "sl:1.0"} {
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

func TestAcceptsRealPackageNames(t *testing.T) {
	for _, name := range []string{"sl", "bash", "gcc-c++", "python3.12", "NetworkManager-tui", "libstdc++"} {
		if err := validName(name); err != nil {
			t.Errorf("validName(%q) = %v", name, err)
		}
	}
}

func TestAcceptsRealVersions(t *testing.T) {
	for _, version := range []string{"5.02-22.fc41", "1.0~rc1-1", "2.0^20240101-3", "1:1.2-3"} {
		if err := validVersion(version); err != nil {
			t.Errorf("validVersion(%q) = %v", version, err)
		}
	}
}

func TestTypesAndName(t *testing.T) {
	p := NewWith(runtest.New())
	if p.Name() != "dnf" {
		t.Errorf("Name = %q", p.Name())
	}
	if types := p.Types(); len(types) != 2 || types[0] != "Package" || types[1] != "Repository" {
		t.Errorf("Types = %v", types)
	}
}

func TestSatisfiesTheProviderInterface(t *testing.T) {
	var _ provider.Provider = NewWith(runtest.New())
}

// dnf5 rejects a `--` separator as an unknown argument, where apt-get accepts it. This
// was an apt assumption that leaked, and the integration test caught it.
func TestDnfIsNotGivenAnEndOfOptionsSeparator(t *testing.T) {
	runner := runtest.New()

	if err := NewWith(runner).Apply(context.Background(), request("sl", nil), state.Create); err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("dnf")
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range argv {
		if arg == "--" {
			t.Errorf("argv = %v, and dnf does not accept --", argv)
		}
	}
}

// rpm does accept one, so the two programs are not assumed to agree.
func TestRpmIsGivenAnEndOfOptionsSeparator(t *testing.T) {
	runner := runtest.New().Output("5.02-22.fc41\n", queryCall("sl")...)

	if _, err := NewWith(runner).Observe(context.Background(), request("sl", nil)); err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("rpm")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, arg := range argv {
		if arg == "--" {
			found = true
		}
	}
	if !found {
		t.Errorf("argv = %v, want a -- separator", argv)
	}
}
