// SPDX-License-Identifier: Apache-2.0

package document

import (
	"strings"
	"testing"
)

func parse(t *testing.T, body string) (Parsed, Errors) {
	t.Helper()
	return ParseFile("fleet/test.yaml", []byte(body))
}

// mustParse fails the test if anything was reported, for cases where the input is
// meant to be valid.
func mustParse(t *testing.T, body string) Parsed {
	t.Helper()
	out, errs := parse(t, body)
	if errs.Len() != 0 {
		t.Fatalf("unexpected errors:\n%v", errs.Error())
	}
	return out
}

// wantError checks that one of the reported problems mentions want.
func wantError(t *testing.T, errs Errors, want string) {
	t.Helper()
	if errs.Len() == 0 {
		t.Fatalf("expected an error mentioning %q, got none", want)
	}
	for _, e := range errs.List() {
		if strings.Contains(e.Msg, want) {
			return
		}
	}
	t.Fatalf("expected an error mentioning %q, got:\n%v", want, errs.Error())
}

func TestParseFleet(t *testing.T) {
	out := mustParse(t, `
datum: v1alpha1
type: Fleet
name: example
exclude:
  - "**/README.md"
`)
	if len(out.Fleets) != 1 {
		t.Fatalf("got %d fleets, want 1", len(out.Fleets))
	}
	if out.Fleets[0].Name != "example" {
		t.Errorf("name = %q, want example", out.Fleets[0].Name)
	}
	if got := out.Fleets[0].Exclude; len(got) != 1 || got[0] != "**/README.md" {
		t.Errorf("exclude = %v", got)
	}
}

func TestParseHostLabels(t *testing.T) {
	out := mustParse(t, `
datum: v1alpha1
type: Host
name: web-001
labels:
  environment: production
  role: web
`)
	host := out.Hosts[0]
	if host.Labels["environment"] != "production" || host.Labels["role"] != "web" {
		t.Errorf("labels = %v", host.Labels)
	}
}

func TestHostCannotSetReservedLabel(t *testing.T) {
	_, errs := parse(t, `
datum: v1alpha1
type: Host
name: web-001
labels:
  datum/host: someone-else
`)
	wantError(t, errs, "reserved")
}

func TestParseLayerWithMatcher(t *testing.T) {
	out := mustParse(t, `
datum: v1alpha1
type: Layer
name: role-web
precedence: 30
match:
  labels:
    role: web
  oneOf:
    site: [london, frankfurt]
  noneOf:
    tier: [legacy]
  has:
    - monitoring
  missing:
    - decommissioned
`)
	layer := out.Layers[0]
	if layer.Precedence != 30 {
		t.Errorf("precedence = %d, want 30", layer.Precedence)
	}
	if layer.Match.Labels["role"] != "web" {
		t.Errorf("labels = %v", layer.Match.Labels)
	}
	if got := layer.Match.OneOf["site"]; len(got) != 2 {
		t.Errorf("oneOf site = %v", got)
	}
	if got := layer.Match.NoneOf["tier"]; len(got) != 1 {
		t.Errorf("noneOf tier = %v", got)
	}
	if len(layer.Match.Has) != 1 || len(layer.Match.Missing) != 1 {
		t.Errorf("has = %v, missing = %v", layer.Match.Has, layer.Match.Missing)
	}
}

func TestLayerPrecedenceDefaultsToZero(t *testing.T) {
	out := mustParse(t, `
datum: v1alpha1
type: Layer
name: base
`)
	if out.Layers[0].Precedence != 0 {
		t.Errorf("precedence = %d, want 0", out.Layers[0].Precedence)
	}
	if !out.Layers[0].Match.IsEmpty() {
		t.Error("a layer with no match block should match every host")
	}
}

func TestPrecedenceHasToBeAnInteger(t *testing.T) {
	_, errs := parse(t, `
datum: v1alpha1
type: Layer
name: base
precedence: "30"
`)
	wantError(t, errs, "precedence has to be an integer")
}

func TestUnknownMatcherFormIsRejected(t *testing.T) {
	_, errs := parse(t, `
datum: v1alpha1
type: Layer
name: base
match:
  anyOf:
    role: [web]
`)
	wantError(t, errs, `no field "anyOf"`)
}

func TestParseResource(t *testing.T) {
	out := mustParse(t, `
datum: v1alpha1
type: File
name: nginx-config
requires:
  - Package[nginx]
desired:
  path: /etc/nginx/nginx.conf
  mode: "0640"
`)
	resource := out.Resources[0]
	if resource.Ref().String() != "File[nginx-config]" {
		t.Errorf("ref = %s", resource.Ref())
	}
	if len(resource.Requires) != 1 || resource.Requires[0].String() != "Package[nginx]" {
		t.Errorf("requires = %v", resource.Requires)
	}
	mode, ok := resource.Desired.Lookup("mode")
	if !ok {
		t.Fatal("desired.mode missing")
	}
	if mode.Scalar != "0640" || !mode.Quoted {
		t.Errorf("mode = %q quoted=%v, want 0640 quoted", mode.Scalar, mode.Quoted)
	}
}

// An unquoted mode is a number as far as YAML is concerned. The parser records
// that so field validation can reject it rather than guessing at the base.
func TestUnquotedModeIsNotMarkedQuoted(t *testing.T) {
	out := mustParse(t, `
datum: v1alpha1
type: File
name: nginx-config
desired:
  path: /etc/nginx/nginx.conf
  mode: 0640
`)
	mode, _ := out.Resources[0].Desired.Lookup("mode")
	if mode.Quoted {
		t.Error("mode was not quoted in the source but is marked quoted")
	}
}

func TestRestartOnAndReloadOnConflict(t *testing.T) {
	_, errs := parse(t, `
datum: v1alpha1
type: Service
name: nginx
restartOn:
  - File[nginx-config]
reloadOn:
  - File[nginx-config]
desired:
  state: running
`)
	wantError(t, errs, "cannot both be declared")
}

func TestBadReferenceIsReported(t *testing.T) {
	_, errs := parse(t, `
datum: v1alpha1
type: Service
name: nginx
requires:
  - nginx
desired:
  state: running
`)
	wantError(t, errs, "not a resource reference")
}

func TestUnrecognisedTypeIsAnError(t *testing.T) {
	_, errs := parse(t, `
datum: v1alpha1
type: Sysctrl
name: forwarding
desired:
  value: "1"
`)
	wantError(t, errs, "unrecognised type")
}

func TestUnsupportedSchemaVersion(t *testing.T) {
	_, errs := parse(t, `
datum: v1beta1
type: Host
name: web-001
`)
	wantError(t, errs, "unsupported schema version")
}

func TestUnknownFieldIsRejected(t *testing.T) {
	_, errs := parse(t, `
datum: v1alpha1
type: Host
name: web-001
lables:
  role: web
`)
	wantError(t, errs, `Host has no field "lables"`)
}

func TestMultipleDocumentsInOneFile(t *testing.T) {
	out := mustParse(t, `
datum: v1alpha1
type: Layer
name: base
---
datum: v1alpha1
type: Package
name: curl
desired:
  state: present
`)
	if len(out.Layers) != 1 || len(out.Resources) != 1 {
		t.Fatalf("got %d layers and %d resources, want 1 and 1", len(out.Layers), len(out.Resources))
	}
}

func TestAliasesAreRejected(t *testing.T) {
	_, errs := parse(t, `
datum: v1alpha1
type: Package
name: curl
desired: &base
  state: present
`)
	// The anchor alone is fine, and only using it as an alias is refused. This input
	// declares one without referring to it, so it parses.
	if errs.Len() != 0 {
		t.Fatalf("unexpected errors:\n%v", errs.Error())
	}
}

func TestErrorsCarryTheFileAndLine(t *testing.T) {
	_, errs := parse(t, `
datum: v1alpha1
type: Host
name: web-001
labels:
  datum/host: nope
`)
	got := errs.List()[0]
	if got.Position.File != "fleet/test.yaml" {
		t.Errorf("file = %q", got.Position.File)
	}
	if got.Position.Line == 0 {
		t.Error("error has no line number")
	}
}
