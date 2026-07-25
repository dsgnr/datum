// SPDX-License-Identifier: Apache-2.0

package run

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func requirePosix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("these tests drive posix utilities")
	}
}

func TestRunCollectsOutput(t *testing.T) {
	requirePosix(t)

	got, err := Exec{}.Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !got.OK() {
		t.Errorf("code = %d, want 0", got.Code)
	}
	if strings.TrimSpace(got.Stdout) != "hello" {
		t.Errorf("stdout = %q", got.Stdout)
	}
	if err := got.Err(); err != nil {
		t.Errorf("Err on a clean exit = %v", err)
	}
}

// A program that ran and exited non-zero is a result rather than an error, because
// whether that is a failure is the caller's question. `systemctl is-active` exits 3
// for a stopped unit, which is an answer.
func TestNonZeroExitIsNotAnError(t *testing.T) {
	requirePosix(t)

	got, err := Exec{}.Run(context.Background(), "false")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got.OK() {
		t.Error("false should not have exited zero")
	}
	if got.Err() == nil {
		t.Error("Err should describe a non-zero exit")
	}
}

func TestErrNamesTheProgramAndTheReason(t *testing.T) {
	err := Result{
		Argv:   []string{"apt-get", "install", "nginx"},
		Code:   100,
		Stderr: "E: Unable to locate package nginx\nsecond line\n",
	}.Err()

	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"apt-get", "100", "Unable to locate package"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
	// One line, because the rest belongs in a log rather than in a plan.
	if strings.Contains(err.Error(), "second line") {
		t.Errorf("error should be one line, got %q", err)
	}
}

func TestMissingProgramIsAnError(t *testing.T) {
	requirePosix(t)

	_, err := Exec{}.Run(context.Background(), "datum-no-such-program")
	if err == nil {
		t.Fatal("want an error for a program that is not there")
	}
}

func TestEmptyArgvIsRefused(t *testing.T) {
	_, err := Exec{}.Run(context.Background())
	if err == nil {
		t.Fatal("want an error for an empty argument vector")
	}
}

// A relative path would resolve against whatever directory the agent was started
// in, which is not something a provider should depend on.
func TestRelativePathIsRefused(t *testing.T) {
	_, err := Exec{}.Run(context.Background(), "./apt-get", "install")
	if err == nil {
		t.Fatal("want an error for a relative program path")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("err = %v", err)
	}
}

// Nothing is inherited, so a provider behaves the same however the agent was
// started, and a PATH in the agent's environment cannot redirect apt-get.
func TestEnvironmentIsFixedRatherThanInherited(t *testing.T) {
	requirePosix(t)
	t.Setenv("PATH", "/tmp/nowhere")
	t.Setenv("DATUM_LEAK", "leaked")

	got, err := Exec{}.Run(context.Background(), "env")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Stdout, "DATUM_LEAK") {
		t.Errorf("the agent's environment leaked into the program:\n%s", got.Stdout)
	}
	if !strings.Contains(got.Stdout, "PATH="+Path) {
		t.Errorf("PATH should be the fixed one, got:\n%s", got.Stdout)
	}
	if !strings.Contains(got.Stdout, "LC_ALL=C") {
		t.Errorf("the locale should be pinned, got:\n%s", got.Stdout)
	}
}

func TestWithAddsEnvironmentVariables(t *testing.T) {
	requirePosix(t)

	runner := Exec{}.With(map[string]string{"DEBIAN_FRONTEND": "noninteractive"})
	got, err := runner.Run(context.Background(), "env")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Stdout, "DEBIAN_FRONTEND=noninteractive") {
		t.Errorf("stdout:\n%s", got.Stdout)
	}
	// The fixed entries survive.
	if !strings.Contains(got.Stdout, "PATH="+Path) {
		t.Errorf("PATH missing:\n%s", got.Stdout)
	}
}

// An argument vector means shell metacharacters are data. This is the whole reason
// providers do not build command lines.
func TestArgumentsAreNotInterpretedByAShell(t *testing.T) {
	requirePosix(t)
	dir := t.TempDir()
	canary := filepath.Join(dir, "canary")

	payload := "nginx; touch " + canary
	got, err := Exec{}.Run(context.Background(), "echo", payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.Stdout) != payload {
		t.Errorf("the argument should have arrived whole, got %q", got.Stdout)
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatal("the embedded command ran, so something handed the argument to a shell")
	}
}

func TestCancellationStopsTheProgram(t *testing.T) {
	requirePosix(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := Exec{}.Run(ctx, "sleep", "10")
	if err == nil {
		t.Fatal("want an error when the context ends")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("the program ran for %v, so the context was not honoured", elapsed)
	}
}

func TestAvailable(t *testing.T) {
	requirePosix(t)

	if !Available("sh") {
		t.Error("sh should be on the fixed path")
	}
	if Available("datum-no-such-program") {
		t.Error("a program that is not there should not be reported as available")
	}
	if Available("") {
		t.Error("an empty name is not a program")
	}
}

func TestRunnerInterfaceIsSatisfied(t *testing.T) {
	var _ Runner = Exec{}
}

func TestExitCodeIsReported(t *testing.T) {
	requirePosix(t)

	got, err := Exec{}.Run(context.Background(), "sh", "-c", "exit 3")
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != 3 {
		t.Errorf("code = %d, want 3", got.Code)
	}
}

func TestStderrIsCapturedSeparately(t *testing.T) {
	requirePosix(t)

	got, err := Exec{}.Run(context.Background(), "sh", "-c", "echo out; echo err >&2")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.Stdout) != "out" {
		t.Errorf("stdout = %q", got.Stdout)
	}
	if strings.TrimSpace(got.Stderr) != "err" {
		t.Errorf("stderr = %q", got.Stderr)
	}
}

func TestResultErrOnMissingDetail(t *testing.T) {
	err := Result{Argv: []string{"systemctl"}, Code: 1}.Err()
	if err == nil || !strings.Contains(err.Error(), "systemctl exited 1") {
		t.Errorf("err = %v", err)
	}
}

func TestRunDoesNotWaitOnStdin(t *testing.T) {
	requirePosix(t)

	done := make(chan error, 1)
	go func() {
		_, err := Exec{}.Run(context.Background(), "cat")
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a program reading stdin hung the pass")
	}
}
