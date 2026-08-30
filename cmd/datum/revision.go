// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/dsgnr/datum/internal/revision"
)

func init() {
	register(command{
		name:    "revision",
		summary: "Show or clear the revision this host has accepted",
		run:     runRevision,
	})
}

func runRevision(e *env, args []string) int {
	if len(args) == 0 {
		e.errorf("usage: datum revision show|clear [--state DIR]\n")
		return exitError
	}
	switch args[0] {
	case "show":
		return runRevisionShow(e, args[1:])
	case "clear":
		return runRevisionClear(e, args[1:])
	default:
		e.errorf("unknown subcommand %q, want show or clear\n", args[0])
		return exitError
	}
}

func runRevisionShow(e *env, args []string) int {
	fs := newFlagSet(e, "revision show")
	stateDir := fs.String("state", defaultStateDir, "directory holding the accepted revision")
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}

	accepted, found, err := revision.Read(*stateDir)
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}
	if !found {
		// Stated explicitly, since a host with no baseline has downgrade protection disarmed
		// and nothing else reports it.
		e.printf("no accepted revision\n")
		e.printf("this host is at first contact, so the next signed revision it sees becomes its baseline\n")
		return exitOK
	}
	e.printf("%s\n", accepted)
	return exitOK
}

func runRevisionClear(e *env, args []string) int {
	fs := newFlagSet(e, "revision clear")
	stateDir := fs.String("state", defaultStateDir, "directory holding the accepted revision")
	confirm := fs.Bool("yes", false, "confirm that downgrade protection should be reset")
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}

	accepted, found, err := revision.Read(*stateDir)
	if err != nil {
		// Reported and not fatal, since an unreadable pointer is one of the states
		// this command exists to recover from.
		e.errorf("%v\n", err)
	}

	// Refused without confirmation on purpose. Clearing puts the host back to trusting
	// whatever it is next shown, which is the one thing the descendant check exists to
	// prevent, so it should not be something a shell history repeats by accident.
	if !*confirm {
		if found {
			e.printf("this host has accepted %s\n\n", accepted)
		}
		e.errorf("clearing the accepted revision disarms downgrade protection until the next pass\n")
		e.errorf("the host will then accept whatever signed revision it is next shown\n")
		e.errorf("re-run with --yes to confirm\n")
		return exitError
	}

	previous, had, err := revision.Clear(*stateDir)
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}
	if had {
		e.printf("cleared %s\n", previous)
	} else {
		e.printf("there was no accepted revision to clear\n")
	}
	e.printf("the next signed revision this host sees becomes its baseline\n")
	return exitOK
}
