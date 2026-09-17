// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
)

// A binary's schema support decides whether it can be pointed at a repository, so it is
// reported rather than being something to infer from a parse failure.
func TestVersionReportsTheSchemaVersionsItReads(t *testing.T) {
	got := invoke("version")
	if got.code != exitOK {
		t.Fatalf("exited %d: %s", got.code, got.all())
	}
	if !strings.Contains(got.out, "schema    "+document.SupportedSchemas()) {
		t.Errorf("datum version did not report the schema versions: %q", got.out)
	}
}

func TestValidateReportsTheSchemaVersionInUse(t *testing.T) {
	got := invoke("validate", "--repo", "../../examples/fleet")
	if got.code != exitOK {
		t.Fatalf("exited %d: %s", got.code, got.all())
	}
	if !strings.Contains(got.out, "schema     "+document.SchemaVersion) {
		t.Errorf("datum validate did not report the schema version: %q", got.out)
	}
}

// One version reads as a count of documents.
func TestSchemaSummaryCountsASingleVersion(t *testing.T) {
	set := document.Set{
		Fleet:     document.Fleet{Schema: "v1alpha1"},
		Hosts:     []document.Host{{Schema: "v1alpha1"}},
		Layers:    []document.Layer{{Schema: "v1alpha1"}},
		Resources: []document.Resource{{Schema: "v1alpha1"}, {Schema: "v1alpha1"}},
	}
	if got, want := schemaSummary(set), "v1alpha1 (5 documents)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Two versions is a migration part-way through, and the point of the line is to show how
// much of it is left.
func TestSchemaSummaryShowsAMigrationInProgress(t *testing.T) {
	set := document.Set{
		Fleet:  document.Fleet{Schema: "v1alpha1"},
		Hosts:  []document.Host{{Schema: "v1beta1"}},
		Layers: []document.Layer{{Schema: "v1beta1"}},
	}
	got := schemaSummary(set)
	for _, want := range []string{"v1alpha1 1", "v1beta1 2", "migration in progress"} {
		if !strings.Contains(got, want) {
			t.Errorf("got %q, missing %q", got, want)
		}
	}
}

// Versions are listed oldest first, so the line reads the same way every run.
func TestSchemaSummaryOrdersVersions(t *testing.T) {
	set := document.Set{
		Hosts: []document.Host{{Schema: "v1beta1"}, {Schema: "v1alpha1"}},
	}
	got := schemaSummary(set)
	if strings.Index(got, "v1alpha1") > strings.Index(got, "v1beta1") {
		t.Errorf("got %q, want v1alpha1 before v1beta1", got)
	}
}

func TestSchemaSummaryWithNothingDeclared(t *testing.T) {
	if got, want := schemaSummary(document.Set{}), "none declared"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A single document counts as one document, not one documents.
func TestSchemaSummaryWithOneDocument(t *testing.T) {
	set := document.Set{Fleet: document.Fleet{Schema: "v1alpha1"}}
	if got, want := schemaSummary(set), "v1alpha1 (1 document)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
