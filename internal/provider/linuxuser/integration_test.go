//go:build integration

// SPDX-License-Identifier: Apache-2.0

// These tests create and remove real accounts and groups, so they change the account
// database on the machine that runs them. They are behind a build tag for that reason,
// and `make test-user` runs them in a throwaway container.
package linuxuser

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/state"
)

const (
	user  = "datumtest"
	group = "datumtestg"
)

func setup(t *testing.T) *Provider {
	t.Helper()
	if !Detect() {
		t.Skip("the shadow utilities are not installed here")
	}
	cleanup()
	t.Cleanup(cleanup)
	return New()
}

func cleanup() {
	exec.Command("userdel", user).Run()
	exec.Command("groupdel", group).Run()
	exec.Command("groupdel", user).Run()
}

func passwdEntry(t *testing.T, name string) (string, bool) {
	t.Helper()
	out, err := exec.Command("getent", "passwd", name).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func TestIntegrationCreateObserveRemoveGroup(t *testing.T) {
	p := setup(t)
	c := context.Background()
	req := request("Group", group, map[string]string{"state": "present", "gid": "4242"})

	before, err := p.Observe(c, req)
	if err != nil {
		t.Fatal(err)
	}
	if before.Exists {
		t.Fatalf("%s exists already", group)
	}

	if err := p.Apply(c, req, state.Create); err != nil {
		t.Fatal(err)
	}
	after, err := p.Observe(c, req)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Exists {
		t.Fatal("the group should exist")
	}
	if v, _ := after.Value("gid"); v.Scalar != "4242" {
		t.Errorf("gid = %q, want 4242", v.Scalar)
	}

	if err := p.Apply(c, req, state.Remove); err != nil {
		t.Fatal(err)
	}
	gone, err := p.Observe(c, req)
	if err != nil {
		t.Fatal(err)
	}
	if gone.Exists {
		t.Error("the group should be gone")
	}
}

func TestIntegrationCreateObserveRemoveUser(t *testing.T) {
	p := setup(t)
	c := context.Background()

	if err := p.Apply(c, request("Group", group, map[string]string{"state": "present"}), state.Create); err != nil {
		t.Fatal(err)
	}

	req := request("User", user, map[string]string{
		"state":        "present",
		"uid":          "4242",
		"primaryGroup": group,
		"home":         "/home/" + user,
		"shell":        "/bin/sh",
		"comment":      "Datum test account",
	})

	if err := p.Apply(c, req, state.Create); err != nil {
		t.Fatal(err)
	}

	got, err := p.Observe(c, req)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exists {
		t.Fatal("the account should exist")
	}
	for field, want := range map[string]string{
		"uid":          "4242",
		"primaryGroup": group,
		"home":         "/home/" + user,
		"shell":        "/bin/sh",
		"comment":      "Datum test account",
	} {
		if v, _ := got.Value(field); v.Scalar != want {
			t.Errorf("%s = %q, want %q", field, v.Scalar, want)
		}
	}

	if err := p.Apply(c, req, state.Remove); err != nil {
		t.Fatal(err)
	}
	if _, ok := passwdEntry(t, user); ok {
		t.Error("the account should be gone")
	}
}

// home is read from the account record, not from the filesystem, so a user whose home
// directory is recorded but missing reports the recorded path.
func TestIntegrationHomeComesFromTheRecordNotTheFilesystem(t *testing.T) {
	p := setup(t)
	c := context.Background()

	req := request("User", user, map[string]string{
		"state": "present",
		"home":  "/home/" + user,
		"shell": "/bin/sh",
	})
	if err := p.Apply(c, req, state.Create); err != nil {
		t.Fatal(err)
	}

	// The provider does not create the directory, which is a Directory resource.
	if out, err := exec.Command("test", "-d", "/home/"+user).CombinedOutput(); err == nil {
		t.Errorf("the home directory should not have been created: %s", out)
	}

	got, err := p.Observe(c, req)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("home"); v.Scalar != "/home/"+user {
		t.Errorf("home = %q, want the recorded path", v.Scalar)
	}
}

// Fields not declared are not managed, so changing the shell leaves the rest alone.
func TestIntegrationUpdateOnlyChangesWhatIsDeclared(t *testing.T) {
	p := setup(t)
	c := context.Background()

	create := request("User", user, map[string]string{
		"state":   "present",
		"home":    "/home/" + user,
		"shell":   "/bin/sh",
		"comment": "original",
	})
	if err := p.Apply(c, create, state.Create); err != nil {
		t.Fatal(err)
	}

	if err := p.Apply(c, request("User", user, map[string]string{"shell": "/bin/bash"}), state.Update); err != nil {
		t.Fatal(err)
	}

	got, err := p.Observe(c, create)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("shell"); v.Scalar != "/bin/bash" {
		t.Errorf("shell = %q, want /bin/bash", v.Scalar)
	}
	if v, _ := got.Value("comment"); v.Scalar != "original" {
		t.Errorf("comment = %q, want it left alone", v.Scalar)
	}
}

// groups replaces, it does not add, so membership of anything else is drift corrected
// away.
func TestIntegrationGroupsReplace(t *testing.T) {
	p := setup(t)
	c := context.Background()

	for _, name := range []string{group, group + "2"} {
		if err := p.Apply(c, request("Group", name, map[string]string{"state": "present"}), state.Create); err != nil {
			t.Fatal(err)
		}
		defer exec.Command("groupdel", name).Run()
	}

	both := withGroups(request("User", user, map[string]string{
		"state": "present", "home": "/home/" + user, "shell": "/bin/sh",
	}), group, group+"2")
	if err := p.Apply(c, both, state.Create); err != nil {
		t.Fatal(err)
	}

	got, err := p.Observe(c, both)
	if err != nil {
		t.Fatal(err)
	}
	groups, _ := got.Value("groups")
	if len(groups.List) != 2 {
		t.Fatalf("groups = %+v, want two", groups.List)
	}

	one := withGroups(request("User", user, nil), group)
	if err := p.Apply(c, one, state.Update); err != nil {
		t.Fatal(err)
	}
	got, err = p.Observe(c, one)
	if err != nil {
		t.Fatal(err)
	}
	groups, _ = got.Value("groups")
	if len(groups.List) != 1 || groups.List[0].Scalar != group {
		t.Errorf("groups = %+v, want just %s", groups.List, group)
	}
}

// The uid is never passed to usermod, so an account with the wrong one keeps it and the
// difference is reported instead.
func TestIntegrationUidIsNotChanged(t *testing.T) {
	p := setup(t)
	c := context.Background()

	create := request("User", user, map[string]string{
		"state": "present", "uid": "4242", "home": "/home/" + user, "shell": "/bin/sh",
	})
	if err := p.Apply(c, create, state.Create); err != nil {
		t.Fatal(err)
	}

	wanted := request("User", user, map[string]string{
		"state": "present", "uid": "4343", "home": "/home/" + user, "shell": "/bin/sh",
	})
	if err := p.Apply(c, wanted, state.Update); err != nil {
		t.Fatal(err)
	}

	got, err := p.Observe(c, wanted)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Value("uid"); v.Scalar != "4242" {
		t.Errorf("uid = %q, want it left at 4242", v.Scalar)
	}
	if got.CanCorrect("uid") {
		t.Error("uid should be reported as uncorrectable")
	}
}

func TestIntegrationObserveMissingAccount(t *testing.T) {
	p := setup(t)

	for _, typeName := range []string{"User", "Group"} {
		got, err := p.Observe(context.Background(), request(typeName, "datum-no-such-thing", nil))
		if err != nil {
			t.Fatalf("%s: err = %v, want nil", typeName, err)
		}
		if got.Exists {
			t.Errorf("%s should not exist", typeName)
		}
	}
}

// A group that is still somebody's primary group cannot be removed, and the action
// fails rather than forcing it.
func TestIntegrationRemovingAPrimaryGroupFails(t *testing.T) {
	p := setup(t)
	c := context.Background()

	if err := p.Apply(c, request("Group", group, map[string]string{"state": "present"}), state.Create); err != nil {
		t.Fatal(err)
	}
	create := request("User", user, map[string]string{
		"state": "present", "primaryGroup": group, "home": "/home/" + user, "shell": "/bin/sh",
	})
	if err := p.Apply(c, create, state.Create); err != nil {
		t.Fatal(err)
	}

	err := p.Apply(c, request("Group", group, map[string]string{"state": "absent"}), state.Remove)
	if err == nil {
		t.Fatal("want an error, because the group is still a primary group")
	}
}

// A second create over an existing account fails instead of quietly doing nothing,
// which is what makes the plan's existence check load bearing.
func TestIntegrationCreateOverAnExistingAccountFails(t *testing.T) {
	p := setup(t)
	c := context.Background()

	req := request("User", user, map[string]string{
		"state": "present", "home": "/home/" + user, "shell": "/bin/sh",
	})
	if err := p.Apply(c, req, state.Create); err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(c, req, state.Create); err == nil {
		t.Error("a second useradd should fail rather than report success")
	}
}
