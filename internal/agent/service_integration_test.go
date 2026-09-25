//go:build integration

// SPDX-License-Identifier: Apache-2.0

// These tests run the agent as a real service under a booted systemd, using the unit
// file taken out of the documentation instead of a copy written here. A unit that only
// exists in a Markdown block is a unit nobody has run, and its mistakes are the ones a
// reader would hit, such as a directive systemd rejects, a type that does not match how
// the process behaves, or a stop that times out.
//
// `make test-systemd` boots the container these need.
package agent_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	docPath     = "../../docs/lifecycle/running.md"
	unitPath    = "/etc/systemd/system/datum.service"
	configPath  = "/etc/datum/agent.yaml"
	serviceName = "datum.service"
)

// fenced pulls a code block out of the documentation by its title, so the test and the
// page cannot drift apart.
func fenced(t *testing.T, path, title string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	pattern := regexp.MustCompile("(?s)```[a-z]+ title=\"" + regexp.QuoteMeta(title) + "\"\n(.*?)```")
	match := pattern.FindSubmatch(data)
	if match == nil {
		t.Fatalf("%s has no block titled %q", path, title)
	}
	return string(match[1])
}

func systemctl(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	return string(out), err
}

func requireSystemd(t *testing.T) {
	t.Helper()
	if _, err := systemctl(t, "is-system-running", "--wait"); err != nil {
		if out, err := systemctl(t, "is-system-running"); err != nil && !strings.Contains(out, "degraded") {
			t.Skipf("systemd is not running this machine: %s", strings.TrimSpace(out))
		}
	}
}

// install writes the unit from the documentation, plus a configuration file with a short
// interval so a test does not wait half an hour for a pass.
func install(t *testing.T, mode string) {
	t.Helper()

	unit := fenced(t, docPath, unitPath)
	// The documented ExecStart names the installed binary, which is the one path adjusted
	// here, so everything else under test is what a reader would use. An absent path is an
	// error and not a no-op, because a unit pointing at a binary nobody built would fail
	// for reasons that look like the agent's.
	installed := "/usr/bin/datum"
	if !strings.Contains(unit, installed) {
		t.Fatalf("the documented unit does not run %s", installed)
	}
	unit = strings.ReplaceAll(unit, installed, mustBuild(t))

	if err := os.MkdirAll("/etc/datum", 0o755); err != nil {
		t.Fatalf("mkdir /etc/datum: %v", err)
	}
	if err := os.MkdirAll("/var/lib/datum", 0o700); err != nil {
		t.Fatalf("mkdir /var/lib/datum: %v", err)
	}
	if err := os.Chmod("/var/lib/datum", 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	config := "host: web-001\n" +
		"source:\n  url: https://git.example.com/fleet.git\n" +
		"trust:\n  require: none\n" +
		"reconciliation:\n  mode: " + mode + "\n  interval: 5s\n  splay: 0s\n  timeout: 4s\n" +
		"metrics:\n  listen: 127.0.0.1:10056\n" +
		"state: /var/lib/datum\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		t.Fatalf("writing unit: %v", err)
	}

	if out, err := systemctl(t, "daemon-reload"); err != nil {
		t.Fatalf("daemon-reload: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		systemctl(t, "stop", serviceName)
		os.Remove(unitPath)
		systemctl(t, "daemon-reload")
		os.RemoveAll("/var/lib/datum/reports")
	})
}

func mustBuild(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "datum")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", binary, "./cmd/datum")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building datum: %v\n%s", err, out)
	}
	return binary
}

func fleetDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../examples/fleet")
	if err != nil {
		t.Fatalf("resolving the example fleet: %v", err)
	}
	return abs
}

// The unit in the documentation has to be one systemd accepts and one that produces a
// service which stays up between passes.
func TestTheDocumentedUnitStartsAndStaysRunning(t *testing.T) {
	requireSystemd(t)
	install(t, "observe")

	if out, err := systemctl(t, "start", serviceName); err != nil {
		t.Fatalf("start: %v\n%s", err, out)
	}
	if out, err := systemctl(t, "is-active", serviceName); err != nil {
		status, _ := systemctl(t, "status", serviceName, "--no-pager")
		t.Fatalf("is-active said %q: %s", strings.TrimSpace(out), status)
	}

	// The agent is resident, so it has to still be running after more than one interval
	// has passed.
	time.Sleep(12 * time.Second)
	if out, err := systemctl(t, "is-active", serviceName); err != nil {
		journal, _ := exec.Command("journalctl", "-u", serviceName, "--no-pager").CombinedOutput()
		t.Fatalf("the service did not stay running (%s): %s", strings.TrimSpace(out), journal)
	}

	// And it has to have actually reconciled, not merely stayed alive.
	journal, err := exec.Command("journalctl", "-u", serviceName, "--no-pager").CombinedOutput()
	if err != nil {
		t.Fatalf("journalctl: %v", err)
	}
	if !strings.Contains(string(journal), "pass ") {
		t.Fatalf("no pass appears in the journal:\n%s", journal)
	}
	if !strings.Contains(string(journal), "serving metrics on 127.0.0.1:10056") {
		t.Errorf("the agent did not report serving metrics:\n%s", journal)
	}
}

// A stop that leaves systemd waiting for TimeoutStopSec and then killing the process is
// a unit that looks fine and takes thirty seconds to restart.
func TestStoppingIsPromptAndClean(t *testing.T) {
	requireSystemd(t)
	install(t, "observe")

	if out, err := systemctl(t, "start", serviceName); err != nil {
		t.Fatalf("start: %v\n%s", err, out)
	}
	time.Sleep(6 * time.Second)

	started := time.Now()
	if out, err := systemctl(t, "stop", serviceName); err != nil {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("stopping took %s, so SIGTERM was not enough", elapsed)
	}

	// A clean stop has to be recorded as such, or restarting the agent leaves a failed
	// unit.
	out, _ := systemctl(t, "show", "-p", "Result", "-p", "ExecMainStatus", serviceName)
	if !strings.Contains(out, "Result=success") {
		t.Errorf("stopping was not clean: %s", strings.TrimSpace(out))
	}
	if !strings.Contains(out, "ExecMainStatus=0") {
		t.Errorf("the agent exited non-zero on a deliberate stop: %s", strings.TrimSpace(out))
	}
}

// The pass lock has to be released when the agent stops, or a restart skips its first
// tick and nothing says why.
func TestTheLockIsReleasedWhenTheServiceStops(t *testing.T) {
	requireSystemd(t)
	install(t, "observe")

	if out, err := systemctl(t, "start", serviceName); err != nil {
		t.Fatalf("start: %v\n%s", err, out)
	}
	time.Sleep(7 * time.Second)
	if out, err := systemctl(t, "stop", serviceName); err != nil {
		t.Fatalf("stop: %v\n%s", err, out)
	}

	// Taken by hand, which only succeeds if the kernel released the flock with the
	// process.
	binary := mustBuild(t)
	cmd := exec.Command(binary, "reconcile", "--host", "web-001",
		"--repo", fleetDir(t), "--mode", "observe", "--state", "/var/lib/datum")
	out, err := cmd.CombinedOutput()
	// Exit 2 is reported drift, which is a pass that ran. Exit 3 would be the lock.
	if code := cmd.ProcessState.ExitCode(); code == 3 {
		t.Fatalf("the lock was still held after the service stopped:\n%s", out)
	} else if code != 0 && code != 2 {
		t.Logf("reconcile exited %d: %s", code, out)
		if err != nil && code == 1 {
			t.Logf("that is a pass failure rather than a lock, which this test does not assert on")
		}
	}
}
