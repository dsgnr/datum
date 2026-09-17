// SPDX-License-Identifier: Apache-2.0

package document

import (
	"strings"
	"testing"
)

func TestSupportsSchemaAcceptsEveryListedVersion(t *testing.T) {
	for _, version := range SchemaVersions {
		if !SupportsSchema(version) {
			t.Errorf("%s is listed in SchemaVersions and is not supported", version)
		}
	}
}

// The version Datum writes has to be one it can read back.
func TestSchemaVersionsIncludesTheVersionDatumWrites(t *testing.T) {
	if !SupportsSchema(SchemaVersion) {
		t.Fatalf("SchemaVersion %s is not in SchemaVersions %v", SchemaVersion, SchemaVersions)
	}
	if SchemaVersions[len(SchemaVersions)-1] != SchemaVersion {
		t.Errorf("SchemaVersions is oldest first, so the last entry should be %s, got %v",
			SchemaVersion, SchemaVersions)
	}
}

func TestSupportsSchemaRefusesAnythingElse(t *testing.T) {
	for _, version := range []string{"", "v1beta1", "v2", "V1ALPHA1", "1alpha1"} {
		if SupportsSchema(version) {
			t.Errorf("%q is not a supported schema version", version)
		}
	}
}

func TestSupportedSchemasListsEveryVersion(t *testing.T) {
	listed := SupportedSchemas()
	for _, version := range SchemaVersions {
		if !strings.Contains(listed, version) {
			t.Errorf("SupportedSchemas() = %q, missing %s", listed, version)
		}
	}
}

// A document is interpreted at the version it declares, so the version has to survive
// parsing rather than being checked and dropped.
func TestParseRecordsTheDeclaredSchemaVersion(t *testing.T) {
	const src = `
datum: v1alpha1
type: Fleet
name: example
---
datum: v1alpha1
type: Host
name: web-001
---
datum: v1alpha1
type: Layer
name: base
---
datum: v1alpha1
type: Package
name: nginx
desired:
  state: present
`
	out, errs := ParseFile("fleet/datum.yaml", []byte(src))
	if errs.Len() != 0 {
		t.Fatalf("unexpected errors: %s", errs.Error())
	}

	if len(out.Fleets) != 1 || out.Fleets[0].Schema != SchemaVersion {
		t.Errorf("Fleet schema = %q, want %s", out.Fleets[0].Schema, SchemaVersion)
	}
	if len(out.Hosts) != 1 || out.Hosts[0].Schema != SchemaVersion {
		t.Errorf("Host schema = %q, want %s", out.Hosts[0].Schema, SchemaVersion)
	}
	if len(out.Layers) != 1 || out.Layers[0].Schema != SchemaVersion {
		t.Errorf("Layer schema = %q, want %s", out.Layers[0].Schema, SchemaVersion)
	}
	if len(out.Resources) != 1 || out.Resources[0].Schema != SchemaVersion {
		t.Errorf("Resource schema = %q, want %s", out.Resources[0].Schema, SchemaVersion)
	}
}

// The error names what this agent does support, so an operator reading it knows whether
// to upgrade the agent or change the document.
func TestUnsupportedSchemaVersionNamesWhatIsSupported(t *testing.T) {
	const src = `
datum: v1beta1
type: Package
name: nginx
desired:
  state: present
`
	out, errs := ParseFile("fleet/roles/web/nginx.yaml", []byte(src))
	if errs.Len() == 0 {
		t.Fatal("expected an error for an unsupported schema version")
	}
	message := errs.Error()
	for _, want := range []string{`"v1beta1"`, SupportedSchemas(), "fleet/roles/web/nginx.yaml"} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not mention %q", message, want)
		}
	}
	if len(out.Resources) != 0 {
		t.Error("a document at an unsupported version should not be parsed")
	}
}
