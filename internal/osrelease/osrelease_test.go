// SPDX-License-Identifier: Apache-2.0

package osrelease

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDebian(t *testing.T) {
	path := write(t, `PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"
NAME="Debian GNU/Linux"
VERSION_ID="12"
ID=debian
HOME_URL="https://www.debian.org/"
`)
	got := ReadFrom(path)
	if got.ID != "debian" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Version != "12" {
		t.Errorf("Version = %q", got.Version)
	}
	if len(got.Like) != 0 {
		t.Errorf("Like = %v, want empty", got.Like)
	}
}

// Ubuntu quotes ID_LIKE and Debian does not, so both forms have to parse.
func TestUbuntu(t *testing.T) {
	path := write(t, `NAME="Ubuntu"
VERSION_ID="24.04"
ID=ubuntu
ID_LIKE=debian
`)
	got := ReadFrom(path)
	if got.ID != "ubuntu" {
		t.Errorf("ID = %q", got.ID)
	}
	if len(got.Like) != 1 || got.Like[0] != "debian" {
		t.Errorf("Like = %v", got.Like)
	}
}

// ID_LIKE is space separated and ordered closest first.
func TestRockyListsSeveralRelatives(t *testing.T) {
	path := write(t, `ID="rocky"
ID_LIKE="rhel centos fedora"
VERSION_ID="9.3"
`)
	got := ReadFrom(path)
	if got.ID != "rocky" {
		t.Errorf("ID = %q", got.ID)
	}
	want := []string{"rhel", "centos", "fedora"}
	if len(got.Like) != len(want) {
		t.Fatalf("Like = %v, want %v", got.Like, want)
	}
	for i := range want {
		if got.Like[i] != want[i] {
			t.Errorf("Like[%d] = %q, want %q", i, got.Like[i], want[i])
		}
	}
}

// VERSION_ID is genuinely absent on rolling releases, so it cannot be required.
func TestArchHasNoVersion(t *testing.T) {
	path := write(t, `ID=arch
NAME="Arch Linux"
`)
	got := ReadFrom(path)
	if got.ID != "arch" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Version != "" {
		t.Errorf("Version = %q, want empty", got.Version)
	}
}

// The specification says to assume linux when ID is unset.
func TestMissingIDDefaultsToLinux(t *testing.T) {
	path := write(t, "NAME=\"Something\"\n")
	if got := ReadFrom(path).ID; got != "linux" {
		t.Errorf("ID = %q, want linux", got)
	}
}

func TestNoFileAtAll(t *testing.T) {
	got := ReadFrom(filepath.Join(t.TempDir(), "absent"))
	if got.ID != "linux" {
		t.Errorf("ID = %q, want linux", got.ID)
	}
}

// The specification is explicit that the two files should not be combined, so the
// first one found is the one used.
func TestFirstFileFoundWins(t *testing.T) {
	first := write(t, "ID=debian\n")
	second := write(t, "ID=fedora\nID_LIKE=rhel\n")

	got := ReadFrom(first, second)
	if got.ID != "debian" {
		t.Errorf("ID = %q, want debian", got.ID)
	}
	if len(got.Like) != 0 {
		t.Errorf("Like = %v, want the second file to have been ignored entirely", got.Like)
	}
}

func TestFallsBackToTheSecondPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	present := write(t, "ID=fedora\n")

	if got := ReadFrom(missing, present).ID; got != "fedora" {
		t.Errorf("ID = %q, want fedora", got)
	}
}

func TestCommentsAndBlankLinesAreIgnored(t *testing.T) {
	path := write(t, `# a comment
ID=debian

#ID=fedora
`)
	if got := ReadFrom(path).ID; got != "debian" {
		t.Errorf("ID = %q", got)
	}
}

func TestSingleQuotesAreStripped(t *testing.T) {
	path := write(t, "ID='alpine'\n")
	if got := ReadFrom(path).ID; got != "alpine" {
		t.Errorf("ID = %q", got)
	}
}

// Identifiers is the order selection tries them in, the host's own identity first and
// then what it claims to resemble.
func TestIdentifiersOrder(t *testing.T) {
	release := Release{ID: "rocky", Like: []string{"rhel", "centos"}}
	got := release.Identifiers()
	want := []string{"rocky", "rhel", "centos"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadUsesTheRealPaths(t *testing.T) {
	// Not asserting on the value, because it depends on where the tests run. The
	// point is that it never panics and always names something.
	if got := Read().ID; got == "" {
		t.Error("ID should never be empty")
	}
}
