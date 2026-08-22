// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/dsgnr/datum/internal/config"
	"github.com/dsgnr/datum/internal/schedule"
	"github.com/dsgnr/datum/internal/statedir"
)

func init() {
	register(command{
		name:    "config",
		summary: "Check the agent configuration file",
		run:     runConfig,
	})
}

func runConfig(e *env, args []string) int {
	if len(args) == 0 || args[0] != "check" {
		e.errorf("usage: datum config check [--config PATH]\n")
		return exitError
	}

	fs := newFlagSet(e, "config check")
	path := fs.String("config", config.Path, "agent configuration file")
	if _, err := parseFlags(fs, args[1:]); err != nil {
		return exitError
	}

	cfg, warnings, err := config.Load(*path)
	if err != nil {
		e.printf("%s\n", *path)
		e.errorf("%v\n", err)
		return exitError
	}

	// Resolved values including defaults, rather than an echo of the file. A key in the
	// wrong place and a setting that is not applied both look correct when the file is
	// read back verbatim.
	e.printf("%s       ok\n", *path)

	w := tabwriter.NewWriter(e.out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  host\t%s\n", cfg.Host)
	fmt.Fprintf(w, "  source.url\t%s\n", orNone(cfg.Source.URL))
	fmt.Fprintf(w, "  source.branch\t%s\n", cfg.Source.Branch)
	if cfg.Source.Credential != "" {
		fmt.Fprintf(w, "  source.credential\t%s\n", cfg.Source.Credential)
	}
	fmt.Fprintf(w, "  trust.require\t%s\n", cfg.Trust.Require)
	fmt.Fprintf(w, "  trust.signers\t%s\n", describeSigners(cfg.Trust.Signers))
	if cfg.Trust.Require == config.RequireSignedTag {
		fmt.Fprintf(w, "  trust.tagPattern\t%s\n", cfg.Trust.TagPattern)
	}
	fmt.Fprintf(w, "  trust.requireDescendant\t%t\n", cfg.Trust.Descendant())
	fmt.Fprintf(w, "  reconciliation.mode\t%s\n", cfg.Reconciliation.Mode)

	// The schedule is reported rather than the interval alone, since the offset is derived
	// from the host name and cannot be worked out by hand.
	s := schedule.New(cfg.Host, time.Duration(cfg.Reconciliation.Interval), cfg.Reconciliation.SplayOr())
	fmt.Fprintf(w, "  reconciliation.interval\t%s, offset %s\n",
		cfg.Reconciliation.Interval, offsetClock(s.Offset()))
	fmt.Fprintf(w, "  reconciliation.timeout\t%s\n", cfg.Reconciliation.Timeout)
	fmt.Fprintf(w, "  secrets.provider\t%s\n", orNone(cfg.Secrets.Provider))
	fmt.Fprintf(w, "  metrics.listen\t%s\n", cfg.Metrics.Listen)
	if cfg.Metrics.Textfile != "" {
		fmt.Fprintf(w, "  metrics.textfile\t%s\n", cfg.Metrics.Textfile)
	}
	fmt.Fprintf(w, "  state\t%s   %s\n", cfg.State, describeStateDir(cfg.State))
	w.Flush()

	if len(warnings) > 0 {
		e.printf("\n")
		for _, warning := range warnings {
			e.printf("  %s\n", warning)
		}
	}
	return exitOK
}

// describeSigners reports the key count, which is enough to confirm the expected file
// was read.
func describeSigners(path string) string {
	if path == "" {
		return "none"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return path + "   not readable"
	}
	keys := 0
	for _, line := range splitLines(string(data)) {
		if line != "" && line[0] != '#' {
			keys++
		}
	}
	return fmt.Sprintf("%s   %s", path, plural(keys, "key"))
}

// describeStateDir reports the permissions, since the agent exits at startup when they
// are wrong.
func describeStateDir(dir string) string {
	info, err := os.Stat(dir)
	if err != nil {
		return "missing, it will be created"
	}
	if err := statedir.Check(dir); err != nil {
		return fmt.Sprintf("%04o   %v", info.Mode().Perm(), err)
	}
	return fmt.Sprintf("root %04o   ok", info.Mode().Perm())
}

// offsetClock prints an offset as minutes and seconds past the interval boundary,
// which is the form the reference page shows.
func offsetClock(offset time.Duration) string {
	rounded := offset.Round(time.Second)
	return fmt.Sprintf("%02d:%02d", int(rounded.Minutes()), int(rounded.Seconds())%60)
}

func orNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func splitLines(text string) []string {
	var out []string
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			out = append(out, trimSpace(text[start:i]))
			start = i + 1
		}
	}
	out = append(out, trimSpace(text[start:]))
	return out
}

func trimSpace(text string) string {
	for len(text) > 0 && (text[0] == ' ' || text[0] == '\t' || text[0] == '\r') {
		text = text[1:]
	}
	for len(text) > 0 {
		last := text[len(text)-1]
		if last != ' ' && last != '\t' && last != '\r' {
			break
		}
		text = text[:len(text)-1]
	}
	return text
}
