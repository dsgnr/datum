// SPDX-License-Identifier: Apache-2.0

package linuxuser

import (
	"context"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/run/runtest"
	"github.com/dsgnr/datum/internal/state"
)

func request(typeName, name string, fields map[string]string) provider.Request {
	desired := document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}
	for k, v := range fields {
		desired.Map[k] = document.Scalar(v)
	}
	return provider.Request{
		Ref:     document.Reference{Type: typeName, Name: name},
		Target:  name,
		Desired: desired,
	}
}

func withGroups(req provider.Request, groups ...string) provider.Request {
	items := make([]document.Value, 0, len(groups))
	for _, group := range groups {
		items = append(items, document.Scalar(group))
	}
	req.Desired.Map["groups"] = document.Value{Kind: document.KindList, List: items}
	return req
}

// A host with one account and the groups it belongs to.
func accountHost() *runtest.Runner {
	return runtest.New().
		Output("deploy:x:1001:1001:Deployment account:/home/deploy:/bin/bash\n",
			"getent", "passwd", "deploy").
		Output("deploy:x:1001:\n", "getent", "group", "1001").
		Output("deploy:x:1001:\n", "getent", "group", "deploy").
		Output("deploy docker\n", "id", "--groups", "--name", "deploy")
}

func TestObserveUser(t *testing.T) {
	got, err := NewWith(accountHost()).Observe(context.Background(),
		request("User", "deploy", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exists {
		t.Fatal("the account should exist")
	}
	for field, want := range map[string]string{
		"uid":          "1001",
		"comment":      "Deployment account",
		"home":         "/home/deploy",
		"shell":        "/bin/bash",
		"primaryGroup": "deploy",
	} {
		if v, _ := got.Value(field); v.Scalar != want {
			t.Errorf("%s = %q, want %q", field, v.Scalar, want)
		}
	}
}

// The primary group is not a supplementary one, so it is left out of the list a
// document's groups field describes.
func TestSupplementaryGroupsExcludeThePrimary(t *testing.T) {
	got, err := NewWith(accountHost()).Observe(context.Background(),
		request("User", "deploy", nil))
	if err != nil {
		t.Fatal(err)
	}
	groups, _ := got.Value("groups")
	if len(groups.List) != 1 || groups.List[0].Scalar != "docker" {
		t.Errorf("groups = %+v, want just docker", groups.List)
	}
}

// getent exits 2 for a name the database does not hold, which is an absent account, not
// a failure.
func TestObserveMissingUser(t *testing.T) {
	runner := runtest.New().Reply(runtest.Reply{Code: 2}, "getent", "passwd", "deploy")

	got, err := NewWith(runner).Observe(context.Background(), request("User", "deploy", nil))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got.Exists {
		t.Error("an account that is not there does not exist")
	}
}

func TestObserveReportsAFailingGetent(t *testing.T) {
	runner := runtest.New().Fail(1, "getent: cannot reach the name service\n",
		"getent", "passwd", "deploy")

	if _, err := NewWith(runner).Observe(context.Background(), request("User", "deploy", nil)); err == nil {
		t.Fatal("a name service failure should not be read as an absent account")
	}
}

func TestObserveGroup(t *testing.T) {
	runner := runtest.New().Output("deploy:x:1001:alice,bob\n", "getent", "group", "deploy")

	got, err := NewWith(runner).Observe(context.Background(), request("Group", "deploy", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exists {
		t.Fatal("the group should exist")
	}
	if v, _ := got.Value("gid"); v.Scalar != "1001" {
		t.Errorf("gid = %q", v.Scalar)
	}
	// Membership is declared on User and only there, so a group reports no members.
	if _, ok := got.Value("members"); ok {
		t.Error("a group should not report members")
	}
}

// Changing a uid would leave every file owned by the old one belonging to nobody, so
// the difference is reported and never acted on.
func TestUidIsUncorrectable(t *testing.T) {
	got, err := NewWith(accountHost()).Observe(context.Background(),
		request("User", "deploy", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.CanCorrect("uid") {
		t.Error("uid should be reported as uncorrectable")
	}
	if !got.CanCorrect("shell") {
		t.Error("shell is an ordinary field")
	}
}

func TestGidIsUncorrectable(t *testing.T) {
	runner := runtest.New().Output("deploy:x:1001:\n", "getent", "group", "deploy")

	got, err := NewWith(runner).Observe(context.Background(), request("Group", "deploy", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.CanCorrect("gid") {
		t.Error("gid should be reported as uncorrectable")
	}
}

func TestCreateUser(t *testing.T) {
	runner := runtest.New()

	req := withGroups(request("User", "deploy", map[string]string{
		"state":        "present",
		"uid":          "1500",
		"primaryGroup": "deploy",
		"home":         "/home/deploy",
		"shell":        "/bin/bash",
		"comment":      "Deployment account",
	}), "docker")

	if err := NewWith(runner).Apply(context.Background(), req, state.Create); err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("useradd")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(argv, " ")
	for _, want := range []string{
		"--uid 1500", "--gid deploy", "--home-dir /home/deploy",
		"--shell /bin/bash", "--comment Deployment account", "--groups docker",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("argv = %v, want %s", argv, want)
		}
	}
	if argv[len(argv)-1] != "deploy" {
		t.Errorf("argv = %v, want the name last", argv)
	}
	// Managing the directory is a Directory resource, so useradd does not make one.
	if !strings.Contains(line, "--no-create-home") {
		t.Errorf("argv = %v, want --no-create-home", argv)
	}
}

// On creation there is nothing to orphan, so a declared uid is used.
func TestCreateUserUsesADeclaredUid(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(),
		request("User", "deploy", map[string]string{"uid": "1500"}), state.Create)
	if err != nil {
		t.Fatal(err)
	}
	argv, _ := runner.Args("useradd")
	if !strings.Contains(strings.Join(argv, " "), "--uid 1500") {
		t.Errorf("argv = %v", argv)
	}
}

// Fields not declared are not managed, so a resource declaring a shell corrects the
// shell and leaves the rest of the account alone.
func TestUpdateOnlyTouchesDeclaredFields(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(),
		request("User", "deploy", map[string]string{"shell": "/bin/sh"}), state.Update)
	if err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("usermod")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(argv, " ")
	if !strings.Contains(line, "--shell /bin/sh") {
		t.Errorf("argv = %v", argv)
	}
	for _, unwanted := range []string{"--uid", "--home-dir", "--comment", "--gid", "--groups"} {
		if strings.Contains(line, unwanted) {
			t.Errorf("argv = %v should not contain %s", argv, unwanted)
		}
	}
}

// A uid is never passed to usermod, because that is the difference Datum refuses to
// act on.
func TestUpdateNeverChangesAUid(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(),
		request("User", "deploy", map[string]string{"uid": "1500"}), state.Update)
	if err != nil {
		t.Fatal(err)
	}
	// Nothing else was declared, so there is nothing for usermod to do at all.
	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("nothing should have run, got %v", calls)
	}
}

// groups replaces, it does not add, so the list given is the whole membership.
func TestGroupsReplaceRatherThanAdd(t *testing.T) {
	runner := runtest.New()

	req := withGroups(request("User", "deploy", nil), "docker", "sudo")
	if err := NewWith(runner).Apply(context.Background(), req, state.Update); err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("usermod")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(argv, " ")
	if !strings.Contains(line, "--groups docker,sudo") {
		t.Errorf("argv = %v", argv)
	}
	// --append would make the field an instruction, not a description.
	if strings.Contains(line, "--append") {
		t.Errorf("argv = %v should not append", argv)
	}
}

// The order a document lists groups in is not drift, so both sides are sorted.
func TestGroupOrderIsNotDrift(t *testing.T) {
	runner := runtest.New().
		Output("deploy:x:1001:1001::/home/deploy:/bin/sh\n", "getent", "passwd", "deploy").
		Output("deploy:x:1001:\n", "getent", "group", "1001").
		Output("deploy:x:1001:\n", "getent", "group", "deploy").
		Output("deploy sudo docker\n", "id", "--groups", "--name", "deploy")

	req := withGroups(request("User", "deploy", nil), "sudo", "docker")
	got, err := NewWith(runner).Observe(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	observed, _ := got.Value("groups")
	wanted, ok := got.Desired.Lookup("groups")
	if !ok {
		t.Fatal("the provider should supply a sorted desired list")
	}
	if len(observed.List) != len(wanted.List) {
		t.Fatalf("observed %+v wanted %+v", observed.List, wanted.List)
	}
	for i := range observed.List {
		if observed.List[i].Scalar != wanted.List[i].Scalar {
			t.Errorf("[%d] observed %q wanted %q", i, observed.List[i].Scalar, wanted.List[i].Scalar)
		}
	}
}

// Removal leaves the home directory, the mail spool and any files elsewhere. A home
// directory that should also go is a Directory declared absent, which puts the deletion
// in the plan where it is visible.
func TestRemoveUserLeavesTheHomeDirectory(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(),
		request("User", "deploy", map[string]string{"state": "absent"}), state.Remove)
	if err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("userdel")
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"--remove", "-r"} {
		for _, arg := range argv {
			if arg == unwanted {
				t.Errorf("argv = %v should not remove the home directory", argv)
			}
		}
	}
}

func TestCreateGroup(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(),
		request("Group", "deploy", map[string]string{"state": "present", "gid": "1500"}), state.Create)
	if err != nil {
		t.Fatal(err)
	}
	argv, err := runner.Args("groupadd")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(argv, " ")
	if !strings.Contains(line, "--gid 1500") || argv[len(argv)-1] != "deploy" {
		t.Errorf("argv = %v", argv)
	}
}

func TestCreateSystemGroup(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(),
		request("Group", "www-data", map[string]string{"state": "present", "system": "true"}), state.Create)
	if err != nil {
		t.Fatal(err)
	}
	argv, _ := runner.Args("groupadd")
	if !strings.Contains(strings.Join(argv, " "), "--system") {
		t.Errorf("argv = %v", argv)
	}
}

// A group that is still somebody's primary group cannot be removed, and the action
// fails rather than forcing it.
func TestRemoveGroupReportsAFailure(t *testing.T) {
	runner := runtest.New().Fail(8, "groupdel: cannot remove the primary group of user 'deploy'\n",
		"groupdel", "deploy")

	err := NewWith(runner).Apply(context.Background(),
		request("Group", "deploy", map[string]string{"state": "absent"}), state.Remove)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "primary group") {
		t.Errorf("err = %v", err)
	}
}

// gid is the only field a group has, and it is not corrected, so an update does nothing.
func TestUpdateGroupDoesNothing(t *testing.T) {
	runner := runtest.New()

	err := NewWith(runner).Apply(context.Background(),
		request("Group", "deploy", map[string]string{"gid": "1500"}), state.Update)
	if err != nil {
		t.Fatal(err)
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("nothing should have run, got %v", calls)
	}
}

func TestNoActionRunsNothing(t *testing.T) {
	runner := runtest.New()
	p := NewWith(runner)

	for _, typeName := range []string{"User", "Group"} {
		for _, action := range []state.Action{state.None, state.Skip} {
			if err := p.Apply(context.Background(), request(typeName, "deploy", nil), action); err != nil {
				t.Fatal(err)
			}
		}
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("nothing should have run, got %v", calls)
	}
}

// The utilities take the name as a positional argument, so one starting with a dash
// would be read as an option, and a colon would corrupt the database it is written to.
func TestRejectsNamesThatAreNotAccountNames(t *testing.T) {
	runner := runtest.New()
	p := NewWith(runner)

	for _, name := range []string{
		"",
		"--system",
		"deploy:x:0:0",
		"deploy; userdel root",
		"../../etc/passwd",
		"deploy user",
	} {
		for _, typeName := range []string{"User", "Group"} {
			if _, err := p.Observe(context.Background(), request(typeName, name, nil)); err == nil {
				t.Errorf("%s Observe accepted %q", typeName, name)
			}
			if err := p.Apply(context.Background(), request(typeName, name, nil), state.Create); err == nil {
				t.Errorf("%s Apply accepted %q", typeName, name)
			}
		}
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("a rejected name should not reach a program, got %v", calls)
	}
}

func TestAcceptsRealAccountNames(t *testing.T) {
	for _, name := range []string{"deploy", "www-data", "systemd-network", "_apt", "user.name", "nixbld$"} {
		if err := validName("user name", name); err != nil {
			t.Errorf("validName(%q) = %v", name, err)
		}
	}
}

func TestTypesAndName(t *testing.T) {
	p := NewWith(runtest.New())
	if p.Name() != "linux-user" {
		t.Errorf("Name = %q", p.Name())
	}
	types := p.Types()
	if len(types) != 2 {
		t.Fatalf("Types = %v", types)
	}
}

func TestRejectsATypeItDoesNotServe(t *testing.T) {
	p := NewWith(runtest.New())
	if _, err := p.Observe(context.Background(), request("Package", "nginx", nil)); err == nil {
		t.Error("want an error for a type this provider does not serve")
	}
	if err := p.Apply(context.Background(), request("Package", "nginx", nil), state.Create); err == nil {
		t.Error("want an error for a type this provider does not serve")
	}
}

func TestSatisfiesTheProviderInterface(t *testing.T) {
	var _ provider.Provider = NewWith(runtest.New())
}

// useradd spells the home directory option --home-dir and usermod spells it --home. The
// two utilities do not share an option table, and the integration test found it.
func TestHomeOptionDiffersBetweenUseraddAndUsermod(t *testing.T) {
	create := runtest.New()
	err := NewWith(create).Apply(context.Background(),
		request("User", "deploy", map[string]string{"home": "/home/deploy"}), state.Create)
	if err != nil {
		t.Fatal(err)
	}
	argv, _ := create.Args("useradd")
	if !contains(argv, "--home-dir") {
		t.Errorf("useradd argv = %v, want --home-dir", argv)
	}

	update := runtest.New()
	err = NewWith(update).Apply(context.Background(),
		request("User", "deploy", map[string]string{"home": "/home/deploy"}), state.Update)
	if err != nil {
		t.Fatal(err)
	}
	argv, _ = update.Args("usermod")
	if !contains(argv, "--home") {
		t.Errorf("usermod argv = %v, want --home", argv)
	}
	if contains(argv, "--home-dir") {
		t.Errorf("usermod argv = %v, and usermod does not know --home-dir", argv)
	}
	// Moving the directory is not what a description of the account record asks for.
	if contains(argv, "--move-home") {
		t.Errorf("usermod argv = %v should not move the directory", argv)
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
