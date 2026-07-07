// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
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

type command struct {
	name    string
	summary string
	run     func(args []string) int
}

var commands []command

func register(c command) {
	commands = append(commands, c)
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage(os.Stdout)
		return exitOK
	}

	name := args[0]
	switch name {
	case "-h", "--help", "help":
		usage(os.Stdout)
		return exitOK
	}

	for _, c := range commands {
		if c.name == name {
			return c.run(args[1:])
		}
	}

	fmt.Fprintf(os.Stderr, "datum: unknown command %q\n\n", name)
	usage(os.Stderr)
	return exitError
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

func usage(out *os.File) {
	fmt.Fprint(out, "usage: datum <command> [flags]\n\n")
	if len(commands) == 0 {
		fmt.Fprint(out, "No commands are wired up yet.\n")
		return
	}
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
