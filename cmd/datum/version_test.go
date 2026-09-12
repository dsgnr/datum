// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/version"
)

// All three spellings are accepted, so none of them has to be guessed.
func TestVersionIsReportedHoweverItIsAsked(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-version"}, {"version"}} {
		got := invoke(args...)
		if got.code != exitOK {
			t.Fatalf("datum %v exited %d: %s", args, got.code, got.all())
		}
		if !strings.HasPrefix(got.out, "datum ") {
			t.Errorf("datum %v reported %q", args, got.out)
		}
		if !strings.Contains(got.out, "platform  ") {
			t.Errorf("datum %v did not name the platform: %q", args, got.out)
		}
	}
}

func TestVersionTakesNoArguments(t *testing.T) {
	got := invoke("version", "--short")
	if got.code != exitError || !strings.Contains(got.err, "usage: datum version") {
		t.Errorf("exited %d with %q", got.code, got.all())
	}
}

// A build that was not told its version says unknown, which is what a `go build` with no
// ldflags produces and what the test binary itself is.
func TestAnUnstampedBuildSaysUnknown(t *testing.T) {
	if version.Version() != "unknown" {
		t.Skipf("this binary was stamped as %q", version.Version())
	}
	if !strings.Contains(version.Report(), "datum unknown") {
		t.Errorf("report was %q", version.Report())
	}
}
