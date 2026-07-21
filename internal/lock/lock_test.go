// SPDX-License-Identifier: Apache-2.0

package lock

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func requireFlock(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the pass lock needs flock")
	}
}

// t.TempDir hands out 0755, which Acquire refuses.
func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAcquireAndRelease(t *testing.T) {
	requireFlock(t)
	dir := stateDir(t)

	held, err := Acquire(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}

	// Releasing has to leave the lock takeable again.
	again, err := Acquire(dir, false)
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	again.Release()
}

func TestAcquireCreatesTheStateDirectory(t *testing.T) {
	requireFlock(t)
	dir := filepath.Join(stateDir(t), "state")

	held, err := Acquire(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	info, err := os.Stat(filepath.Join(dir, "pass.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("lock file mode = %o, want 600", info.Mode().Perm())
	}
}

// The second caller fails immediately rather than waiting, because an interactive
// command that blocks silently is indistinguishable from one that has hung.
func TestSecondAcquireFailsFast(t *testing.T) {
	requireFlock(t)
	dir := stateDir(t)

	first, err := Acquire(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	// A separate process, because flock is per open file description and two
	// Acquire calls in one process would not contend.
	out := runHelper(t, dir, "nowait")
	if out.code == 0 {
		t.Fatalf("the second caller should have failed, output: %s", out.text)
	}
	if !strings.Contains(out.text, "another pass is running") {
		t.Errorf("output = %q", out.text)
	}
}

func TestHeldNamesTheHolder(t *testing.T) {
	requireFlock(t)
	dir := stateDir(t)

	first, err := Acquire(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	out := runHelper(t, dir, "nowait")
	if !strings.Contains(out.text, "pid ") {
		t.Errorf("the error should name the holder, got %q", out.text)
	}
}

func TestErrHeldIsMatchable(t *testing.T) {
	err := error(Held{PID: 42})
	if !errors.Is(err, ErrHeld) {
		t.Error("Held should match ErrHeld")
	}
}

// With wait set, the second caller blocks until the first lets go.
func TestWaitBlocksUntilReleased(t *testing.T) {
	requireFlock(t)
	dir := stateDir(t)

	first, err := Acquire(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan result, 1)
	go func() { done <- runHelper(t, dir, "wait") }()

	// Long enough that the helper is certainly blocked on the lock.
	time.Sleep(200 * time.Millisecond)
	select {
	case out := <-done:
		t.Fatalf("the waiting caller returned early: %s", out.text)
	default:
	}

	first.Release()

	select {
	case out := <-done:
		if out.code != 0 {
			t.Errorf("the waiting caller should have succeeded, got %s", out.text)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiting caller never acquired the lock")
	}
}

func TestReleaseOnNilIsSafe(t *testing.T) {
	var held *Lock
	if err := held.Release(); err != nil {
		t.Errorf("Release on nil = %v", err)
	}
}

// A lock is dropped by the kernel when the holder exits, so a killed pass leaves
// nothing to clean up.
func TestLockIsReleasedWhenTheHolderExits(t *testing.T) {
	requireFlock(t)
	dir := stateDir(t)

	out := runHelper(t, dir, "nowait")
	if out.code != 0 {
		t.Fatalf("the helper should have taken the lock: %s", out.text)
	}

	held, err := Acquire(dir, false)
	if err != nil {
		t.Fatalf("the lock should be free after the holder exited: %v", err)
	}
	held.Release()
}

// The helper runs as a child process. flock is per open file description, so two
// Acquire calls inside one process would not contend and the tests would pass for
// the wrong reason.
const (
	helperEnv = "DATUM_LOCK_HELPER"
	dirEnv    = "DATUM_LOCK_DIR"
)

type result struct {
	code int
	text string
}

func TestMain(m *testing.M) {
	mode := os.Getenv(helperEnv)
	if mode == "" {
		os.Exit(m.Run())
	}
	os.Exit(helperMain(mode, os.Getenv(dirEnv)))
}

func helperMain(mode, dir string) int {
	held, err := Acquire(dir, mode == "wait")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Left for the parent to release by exiting, in the nowait case.
	if mode == "wait" {
		held.Release()
	}
	return 0
}

func runHelper(t *testing.T, dir, mode string) result {
	t.Helper()

	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperEnv+"="+mode, dirEnv+"="+dir)
	out, err := cmd.CombinedOutput()

	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("running the helper: %v", err)
	}
	return result{code: code, text: string(out)}
}
