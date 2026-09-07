// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
)

// Exit codes are part of the command line contract, so they are named.
const (
	exitOK       = 0
	exitError    = 1
	exitDiffers  = 2
	exitLockHeld = 3
)

// env is where a command writes, so a test can run one and read the output.
type env struct {
	out io.Writer
	err io.Writer
}

func (e *env) printf(format string, args ...any) {
	fmt.Fprintf(e.out, format, args...)
}

func (e *env) errorf(format string, args ...any) {
	fmt.Fprintf(e.err, format, args...)
}

type command struct {
	name    string
	summary string
	run     func(e *env, args []string) int
}

var commands []command

func register(c command) {
	commands = append(commands, c)
}

func main() {
	e := &env{out: os.Stdout, err: os.Stderr}
	os.Exit(run(e, os.Args[1:]))
}

func run(e *env, args []string) int {
	if len(args) == 0 {
		usage(e.out)
		return exitOK
	}

	name := args[0]
	switch name {
	case "-h", "--help", "help":
		usage(e.out)
		return exitOK
	case "-version", "--version":
		e.printf("%s", versionReport())
		return exitOK
	}

	for _, c := range commands {
		if c.name == name {
			return c.run(e, args[1:])
		}
	}

	e.errorf("datum: unknown command %q\n\n", name)
	usage(e.err)
	return exitError
}

// newFlagSet reports problems through the command's own writer.
func newFlagSet(e *env, name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(e.err)
	return fs
}

// parseFlags handles flags before or after positional arguments, which flag.Parse will
// not do, because it stops at the first argument that is not a flag. The documented
// usage is "datum explain File[x] --host y".
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
}

func usage(out io.Writer) {
	fmt.Fprint(out, "usage: datum <command> [flags]\n\n")
	sorted := append([]command(nil), commands...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].name < sorted[j].name })

	width := 0
	for _, c := range sorted {
		if len(c.name) > width {
			width = len(c.name)
		}
	}
	for _, c := range sorted {
		fmt.Fprintf(out, "  %-*s  %s\n", width, c.name, c.summary)
	}
}
