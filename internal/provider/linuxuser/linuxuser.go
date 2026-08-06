// SPDX-License-Identifier: Apache-2.0

// Package linuxuser implements User and Group through the shadow utilities.
//
// Reading goes through getent, which answers from whatever name service the host is
// configured with instead of only /etc/passwd. Changing goes through useradd, usermod,
// userdel, groupadd, groupmod and groupdel.
//
// This is not distribution specific. The account database and the utilities that edit
// it are the same interface everywhere the design targets, which is why the provider
// claims no distribution and is selected wherever the utilities exist.
package linuxuser

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/run"
	"github.com/dsgnr/datum/internal/state"
)

// Provider satisfies User and Group.
type Provider struct {
	runner run.Runner
}

func New() *Provider {
	return &Provider{runner: run.Exec{}}
}

// NewWith returns a provider driving a supplied runner, which is how the tests run
// without touching the account database.
func NewWith(runner run.Runner) *Provider {
	return &Provider{runner: runner}
}

func (p *Provider) Name() string    { return "linux-user" }
func (p *Provider) Types() []string { return []string{"User", "Group"} }

// Detect reports whether the account utilities are here. They are absent from some
// minimal images, where the resources are better skipped than failed one at a time.
func Detect() bool {
	for _, program := range []string{"getent", "useradd", "usermod", "groupadd"} {
		if !run.Available(program) {
			return false
		}
	}
	return true
}

func (p *Provider) Observe(ctx context.Context, req provider.Request) (provider.Observation, error) {
	switch req.Ref.Type {
	case "User":
		return p.observeUser(ctx, req)
	case "Group":
		return p.observeGroup(ctx, req)
	default:
		return provider.Observation{}, fmt.Errorf("linuxuser: %s is not a type this provider serves", req.Ref.Type)
	}
}

func (p *Provider) Apply(ctx context.Context, req provider.Request, action state.Action) error {
	switch req.Ref.Type {
	case "User":
		return p.applyUser(ctx, req, action)
	case "Group":
		return p.applyGroup(ctx, req, action)
	default:
		return fmt.Errorf("linuxuser: %s is not a type this provider serves", req.Ref.Type)
	}
}

// getent reads one entry. A name the database does not hold is an absent target, not an
// error, which getent signals with exit 2.
func (p *Provider) getent(ctx context.Context, database, name string) (string, bool, error) {
	result, err := p.runner.Run(ctx, "getent", database, name)
	if err != nil {
		return "", false, err
	}
	switch {
	case result.OK():
		return strings.TrimRight(result.Stdout, "\n"), true, nil
	case result.Code == 2:
		return "", false, nil
	default:
		return "", false, result.Err()
	}
}

// observeUser reads the account record.
func (p *Provider) observeUser(ctx context.Context, req provider.Request) (provider.Observation, error) {
	if err := validName("user name", req.Target); err != nil {
		return provider.Observation{}, err
	}

	line, found, err := p.getent(ctx, "passwd", req.Target)
	if err != nil {
		return provider.Observation{}, err
	}
	if !found {
		return provider.Observation{Exists: false}, nil
	}

	// name:password:uid:gid:comment:home:shell
	fields := strings.Split(line, ":")
	if len(fields) < 7 {
		return provider.Observation{}, fmt.Errorf("linuxuser: cannot read the passwd entry for %s", req.Target)
	}

	primary, err := p.groupName(ctx, fields[3])
	if err != nil {
		return provider.Observation{}, err
	}
	groups, err := p.supplementaryGroups(ctx, req.Target, fields[3])
	if err != nil {
		return provider.Observation{}, err
	}

	observation := provider.Observation{
		Exists: true,
		Fields: map[string]document.Value{
			"uid":          document.Scalar(fields[2]),
			"comment":      document.Scalar(fields[4]),
			"home":         document.Scalar(fields[5]),
			"shell":        document.Scalar(fields[6]),
			"primaryGroup": document.Scalar(primary),
			"groups":       groupList(groups),
		},
		// Changing a uid would leave every file owned by the old one belonging to
		// nobody, and the manifest does not say which files to reassign. The
		// difference is reported on every pass and never acted on.
		Uncorrectable: []string{"uid"},
	}
	// Membership is a set, so the order a document happens to list groups in is not
	// drift. Both sides are sorted before they are compared.
	if declared, ok := req.Desired.Lookup("groups"); ok {
		normalised := req.Desired.Clone()
		normalised.Map["groups"] = groupList(sorted(listValues(declared)))
		observation.Desired = normalised
	}
	return observation, nil
}

func sorted(items []string) []string {
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}

// observeGroup reads the group record.
func (p *Provider) observeGroup(ctx context.Context, req provider.Request) (provider.Observation, error) {
	if err := validName("group name", req.Target); err != nil {
		return provider.Observation{}, err
	}

	line, found, err := p.getent(ctx, "group", req.Target)
	if err != nil {
		return provider.Observation{}, err
	}
	if !found {
		return provider.Observation{Exists: false}, nil
	}

	// name:password:gid:members
	fields := strings.Split(line, ":")
	if len(fields) < 3 {
		return provider.Observation{}, fmt.Errorf("linuxuser: cannot read the group entry for %s", req.Target)
	}

	return provider.Observation{
		Exists: true,
		Fields: map[string]document.Value{
			"gid": document.Scalar(fields[2]),
		},
		// A gid change orphans the group ownership of every file that refers to it,
		// for the same reason a uid change does.
		Uncorrectable: []string{"gid"},
	}, nil
}

// groupName turns a gid into its name, so that primaryGroup compares against what a
// document declares instead of a number.
func (p *Provider) groupName(ctx context.Context, gid string) (string, error) {
	line, found, err := p.getent(ctx, "group", gid)
	if err != nil {
		return "", err
	}
	if !found {
		// A primary group with no entry is a broken account, not a missing one, and the
		// number is more use than an empty string.
		return gid, nil
	}
	if name, _, ok := strings.Cut(line, ":"); ok {
		return name, nil
	}
	return gid, nil
}

// supplementaryGroups lists the groups the account belongs to other than its primary
// one, because that is what a document's groups field describes.
func (p *Provider) supplementaryGroups(ctx context.Context, name, primaryGID string) ([]string, error) {
	result, err := p.runner.Run(ctx, "id", "--groups", "--name", name)
	if err != nil {
		return nil, err
	}
	if !result.OK() {
		return nil, result.Err()
	}

	primary, err := p.groupName(ctx, primaryGID)
	if err != nil {
		return nil, err
	}

	var out []string
	for _, group := range strings.Fields(result.Stdout) {
		if group != primary {
			out = append(out, group)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (p *Provider) applyUser(ctx context.Context, req provider.Request, action state.Action) error {
	if err := validName("user name", req.Target); err != nil {
		return err
	}

	switch action {
	case state.Create:
		return p.run(ctx, append([]string{"useradd"}, p.userArgs(req, true)...)...)
	case state.Update:
		args := p.userArgs(req, false)
		if len(args) == 0 {
			// Only the uid differed, and that is not something to act on.
			return nil
		}
		return p.run(ctx, append([]string{"usermod"}, append(args, req.Target)...)...)
	case state.Remove:
		// The home directory, mail spool and any files elsewhere are left. A home
		// directory that should also go is a Directory resource declared absent,
		// which puts the deletion in the plan where it is visible.
		return p.run(ctx, "userdel", req.Target)
	case state.None, state.Skip:
		return nil
	default:
		return fmt.Errorf("linuxuser: unexpected action %s", action)
	}
}

// userArgs builds the options for useradd or usermod from the declared fields.
//
// Fields not declared are not managed, so nothing appears for them and the account
// keeps whatever it has. The uid is never included, because a uid change is reported
// and not corrected.
func (p *Provider) userArgs(req provider.Request, creating bool) []string {
	var args []string
	if value, ok := req.Field("comment"); ok {
		args = append(args, "--comment", value)
	}
	if value, ok := req.Field("home"); ok {
		// useradd spells this --home-dir and usermod spells it --home, for the same
		// field. The two utilities do not share an option table.
		//
		// Neither is told to move or create the directory, because home is read from
		// the account record and managing the directory is a Directory resource.
		if creating {
			args = append(args, "--home-dir", value)
		} else {
			args = append(args, "--home", value)
		}
	}
	if value, ok := req.Field("shell"); ok {
		args = append(args, "--shell", value)
	}
	if value, ok := req.Field("primaryGroup"); ok {
		args = append(args, "--gid", value)
	}
	if groups, ok := req.Desired.Lookup("groups"); ok {
		// groups replaces, it does not add, so the list given is the whole membership and
		// usermod is told to set it.
		args = append(args, "--groups", strings.Join(listValues(groups), ","))
	}
	if creating {
		if uid, ok := req.Field("uid"); ok {
			// On creation there is nothing to orphan, so a declared uid is used.
			args = append(args, "--uid", uid)
		}
		if req.FieldOr("system", "false") == "true" {
			args = append(args, "--system")
		}
		args = append(args, "--no-create-home", req.Target)
	}
	return args
}

func (p *Provider) applyGroup(ctx context.Context, req provider.Request, action state.Action) error {
	if err := validName("group name", req.Target); err != nil {
		return err
	}

	switch action {
	case state.Create:
		args := []string{"groupadd"}
		if gid, ok := req.Field("gid"); ok {
			args = append(args, "--gid", gid)
		}
		if req.FieldOr("system", "false") == "true" {
			args = append(args, "--system")
		}
		return p.run(ctx, append(args, req.Target)...)
	case state.Update:
		// gid is the only field a group has, and it is not corrected.
		return nil
	case state.Remove:
		// A group that is still somebody's primary group cannot be removed, and the action
		// fails rather than forcing it.
		return p.run(ctx, "groupdel", req.Target)
	case state.None, state.Skip:
		return nil
	default:
		return fmt.Errorf("linuxuser: unexpected action %s", action)
	}
}

func (p *Provider) run(ctx context.Context, argv ...string) error {
	result, err := p.runner.Run(ctx, argv...)
	if err != nil {
		return err
	}
	return result.Err()
}

// groupList renders the observed groups the way a document declares them, so the two
// compare as the same shape.
func groupList(groups []string) document.Value {
	items := make([]document.Value, 0, len(groups))
	for _, group := range groups {
		items = append(items, document.Scalar(group))
	}
	return document.Value{Kind: document.KindList, List: items}
}

func listValues(value document.Value) []string {
	if value.Kind != document.KindList {
		return nil
	}
	out := make([]string, 0, len(value.List))
	for _, item := range value.List {
		out = append(out, item.Scalar)
	}
	return out
}

// validName rejects anything that is not a user or group name. The utilities take the
// name as a positional argument, so one starting with a dash would be read as an
// option, and one containing a colon would corrupt the database it is written into.
func validName(kind, name string) error {
	if err := run.Word(kind, name, "-_.$"); err != nil {
		return fmt.Errorf("linuxuser: %w", err)
	}
	return nil
}
