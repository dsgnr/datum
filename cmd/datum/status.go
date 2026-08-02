// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/dsgnr/datum/internal/report"
)

func init() {
	register(command{
		name:    "status",
		summary: "Show what the last pass did on this host",
		run:     runStatus,
	})
}

func runStatus(e *env, args []string) int {
	fs := newFlagSet(e, "status")
	stateDir := fs.String("state", defaultStateDir, "directory the reports were written to")
	format := fs.String("output", "text", "text or json")
	resources := fs.Bool("resources", false, "list every resource rather than a summary")
	if _, err := parseFlags(fs, args); err != nil {
		return exitError
	}

	// Checked before the report is read, so a typo is reported whether or not a pass
	// has ever run.
	if *format != "text" && *format != "json" {
		e.errorf("unknown output %q, want text or json\n", *format)
		return exitError
	}

	r, found, err := report.Latest(*stateDir)
	if err != nil {
		e.errorf("%v\n", err)
		return exitError
	}
	if !found {
		// A host that has never run is not a host in trouble, so this is not an
		// error. It is also not nothing, which is why it says so.
		e.printf("no pass has run on this host yet\n")
		return exitOK
	}

	if *format == "json" {
		body, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			e.errorf("%v\n", err)
			return exitError
		}
		e.printf("%s\n", body)
	} else {
		printStatus(e, r, *resources)
	}

	return codeForState(r)
}

func printStatus(e *env, r report.Report, withResources bool) {
	w := tabwriter.NewWriter(e.out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "host\t%s\n", r.Host)
	fmt.Fprintf(w, "\ndesired\n")
	fmt.Fprintf(w, "  revisionAttempted\t%s\n", or(r.RevisionAttempted, "unknown"))
	fmt.Fprintf(w, "  revisionApplied\t%s\n", or(r.RevisionApplied, "none"))
	if r.LastKnownGood != "" {
		fmt.Fprintf(w, "  lastKnownGood\t%s\n", r.LastKnownGood)
	}
	if r.Manifest != "" {
		fmt.Fprintf(w, "  manifest\t%s\n", r.Manifest)
	}
	if r.Error != "" {
		fmt.Fprintf(w, "  error\t%s\n", r.Error)
	}

	fmt.Fprintf(w, "\nstate\n")
	fmt.Fprintf(w, "  condition\t%s\n", r.HostState)
	if r.Mode != "" {
		fmt.Fprintf(w, "  mode\t%s\n", r.Mode)
	}

	fmt.Fprintf(w, "\nlast pass\n")
	fmt.Fprintf(w, "  outcome\t%s\n", r.Outcome)
	fmt.Fprintf(w, "  finished\t%s (%s ago)\n",
		r.FinishedAt.Format(time.RFC3339), since(r.FinishedAt))
	fmt.Fprintf(w, "  duration\t%dms\n", r.DurationMS)

	fmt.Fprintf(w, "\nresources\n")
	fmt.Fprintf(w, "  total\t%d\n", r.Counts.Total)
	fmt.Fprintf(w, "  converged\t%d\n", r.Counts.Converged)
	fmt.Fprintf(w, "  drifted\t%d\n", r.Counts.Drifted)
	fmt.Fprintf(w, "  failed\t%d\n", r.Counts.Failed)
	fmt.Fprintf(w, "  blocked\t%d\n", r.Counts.Blocked)
	fmt.Fprintf(w, "  skipped\t%d\n", r.Counts.Skipped)
	w.Flush()

	// The ones that matter are shown either way. A host with nothing wrong prints no
	// resource list at all.
	e.printf("\n")
	detailed := tabwriter.NewWriter(e.out, 0, 0, 2, ' ', 0)
	for _, resource := range r.Resources {
		if !withResources && resource.State == "converged" {
			continue
		}
		fmt.Fprintf(detailed, "%-10s %s\t%s\n", resource.State, resource.Ref, statusDetail(resource))
	}
	detailed.Flush()
}

func statusDetail(resource report.Resource) string {
	switch {
	case resource.Error != "":
		return resource.Error
	case resource.Reason != "":
		return resource.Reason
	case len(resource.Fields) > 0:
		return resource.Action + " " + join(resource.Fields)
	default:
		return resource.Action + " " + resource.Target
	}
}

// codeForState reports the host's condition, not whether reading the report worked,
// because that is the question anybody running this in a check is asking.
func codeForState(r report.Report) int {
	switch r.HostState {
	case "failed":
		return exitError
	case "drifted":
		return exitDiffers
	default:
		// A degraded host exits zero, matching plan and diff. Whether a skipped resource
		// should be non-zero is still open.
		return exitOK
	}
}

func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func since(t time.Time) string {
	d := time.Since(t).Round(time.Second)
	if d < 0 {
		return "0s"
	}
	return d.String()
}
