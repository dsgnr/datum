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
//
// A second Acquire in the same process contends properly, since an flock belongs to
// the open file description rather than to the process.
func TestSecondAcquireFailsFast(t *testing.T) {
	requireFlock(t)
	dir := stateDir(t)

	first, err := Acquire(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	second, err := Acquire(dir, false)
	if err == nil {
		second.Release()
		t.Fatal("the second caller should have been refused")
	}
	if !errors.Is(err, ErrHeld) {
		t.Errorf("err = %v, want ErrHeld", err)
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

	_, err = Acquire(dir, false)
	if err == nil {
		t.Fatal("the second caller should have been refused")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("pid %d", os.Getpid())) {
		t.Errorf("the error should name the holder, got %q", err)
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

	done := make(chan error, 1)
	go func() {
		second, err := Acquire(dir, true)
		if second != nil {
			second.Release()
		}
		done <- err
	}()

	// Long enough that the waiter is certainly blocked on the lock.
	time.Sleep(200 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("the waiting caller returned early: %v", err)
	default:
	}

	first.Release()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the waiting caller should have succeeded, got %v", err)
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
// nothing to clean up. This one needs a real process, because the holder has to die.
func TestLockIsReleasedWhenTheHolderExits(t *testing.T) {
	requireFlock(t)
	dir := stateDir(t)

	if out, code := runHolder(t, dir); code != 0 {
		t.Fatalf("the holder should have taken the lock: %s", out)
	}

	held, err := Acquire(dir, false)
	if err != nil {
		t.Fatalf("the lock should be free after the holder exited: %v", err)
	}
	held.Release()
}

// The helper re-runs this test binary with an environment variable set, which is the
// cheapest way to get a process that takes the lock and then dies.
const (
	helperEnv = "DATUM_LOCK_HELPER"
	dirEnv    = "DATUM_LOCK_DIR"
)

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "" {
		os.Exit(m.Run())
	}
	if _, err := Acquire(os.Getenv(dirEnv), false); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Exiting without releasing is the point.
	os.Exit(0)
}

func runHolder(t *testing.T, dir string) (string, int) {
	t.Helper()

	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperEnv+"=1", dirEnv+"="+dir)
	out, err := cmd.CombinedOutput()

	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return string(out), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("running the holder: %v", err)
	}
	return string(out), 0
}
