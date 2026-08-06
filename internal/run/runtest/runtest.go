// SPDX-License-Identifier: Apache-2.0

// Package runtest is a recording fake for the program runner, so a provider can be
// tested without the programs it drives or the system they belong to.
package runtest

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/dsgnr/datum/internal/run"
)

// Reply is what the fake returns for one command.
type Reply struct {
	Code   int
	Stdout string
	Stderr string
	// Err makes the program fail to run at all, as a missing binary would.
	Err error
}

// Runner answers from a script keyed by the command line, and records what it was
// asked to run.
type Runner struct {
	mu      sync.Mutex
	replies map[string]Reply
	calls   [][]string

	// Default answers anything not in the script. The zero value is a clean exit,
	// which keeps a test that only cares about one command short.
	Default Reply
}

func New() *Runner {
	return &Runner{replies: map[string]Reply{}}
}

// key is the whole argument vector, so a test pins the exact call and not just the
// program name. Getting the arguments wrong is the mistake to catch.
func key(argv []string) string { return strings.Join(argv, " ") }

// Reply scripts one command.
func (r *Runner) Reply(reply Reply, argv ...string) *Runner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replies[key(argv)] = reply
	return r
}

// Output scripts a command that succeeds with the given stdout.
func (r *Runner) Output(stdout string, argv ...string) *Runner {
	return r.Reply(Reply{Stdout: stdout}, argv...)
}

// Fail scripts a command that exits non-zero.
func (r *Runner) Fail(code int, stderr string, argv ...string) *Runner {
	return r.Reply(Reply{Code: code, Stderr: stderr}, argv...)
}

func (r *Runner) Run(ctx context.Context, argv ...string) (run.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return run.Result{Argv: argv}, err
	}
	r.calls = append(r.calls, argv)

	reply, ok := r.replies[key(argv)]
	if !ok {
		reply = r.Default
	}
	if reply.Err != nil {
		return run.Result{Argv: argv}, reply.Err
	}
	return run.Result{
		Argv:   argv,
		Code:   reply.Code,
		Stdout: reply.Stdout,
		Stderr: reply.Stderr,
	}, nil
}

// Calls returns the command lines that were run, in order.
func (r *Runner) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.calls))
	for _, argv := range r.calls {
		out = append(out, key(argv))
	}
	return out
}

// Ran reports whether a command line was run.
func (r *Runner) Ran(argv ...string) bool {
	want := key(argv)
	for _, got := range r.Calls() {
		if got == want {
			return true
		}
	}
	return false
}

// Args returns the argument vector of the first call to a program, so a test can
// assert on arguments without writing the whole line.
func (r *Runner) Args(program string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, argv := range r.calls {
		if len(argv) > 0 && argv[0] == program {
			return argv, nil
		}
	}
	return nil, fmt.Errorf("%s was not run", program)
}
