// SPDX-License-Identifier: Apache-2.0

package document

import (
	"strings"
	"testing"
)

func canonical(v Value) string {
	var b strings.Builder
	v.Canonical(&b)
	return b.String()
}

func TestCanonicalSortsMapKeys(t *testing.T) {
	a := Value{Kind: KindMap, Map: map[string]Value{
		"owner": Scalar("root"),
		"mode":  Scalar("0640"),
		"path":  Scalar("/etc/nginx/nginx.conf"),
	}}
	b := Value{Kind: KindMap, Map: map[string]Value{
		"path":  Scalar("/etc/nginx/nginx.conf"),
		"mode":  Scalar("0640"),
		"owner": Scalar("root"),
	}}
	if canonical(a) != canonical(b) {
		t.Errorf("key order changed the output:\n%s\n%s", canonical(a), canonical(b))
	}
	want := `{"mode":"0640","owner":"root","path":"/etc/nginx/nginx.conf"}`
	if got := canonical(a); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// Provenance is not part of the canonical form. Moving a resource between layers
// changes where a value came from without changing what happens on the host.
func TestCanonicalIgnoresProvenance(t *testing.T) {
	plain := Value{Kind: KindMap, Map: map[string]Value{"mode": Scalar("0600")}}
	traced := Value{Kind: KindMap, Map: map[string]Value{"mode": {
		Kind:      KindScalar,
		Scalar:    "0600",
		From:      "roles/web",
		Displaced: []Origin{{Layer: "base", Scalar: "0644"}},
	}}}
	if canonical(plain) != canonical(traced) {
		t.Errorf("provenance leaked into the canonical form:\n%s\n%s", canonical(plain), canonical(traced))
	}
}

// A quoted "1" and a bare 1 mean the same thing once field validation has
// accepted them, so they must not produce different digests.
func TestCanonicalIgnoresQuoting(t *testing.T) {
	quoted := Value{Kind: KindScalar, Scalar: "1", Quoted: true}
	bare := Value{Kind: KindScalar, Scalar: "1"}
	if canonical(quoted) != canonical(bare) {
		t.Error("quoting changed the canonical form")
	}
}

func TestCanonicalKeepsListOrder(t *testing.T) {
	v := Value{Kind: KindList, List: []Value{Scalar("b"), Scalar("a")}}
	if got, want := canonical(v), `["b","a"]`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestCanonicalEscapes(t *testing.T) {
	v := Scalar(`a"b\c`)
	if got, want := canonical(v), `"a\"b\\c"`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestLookup(t *testing.T) {
	v := Value{Kind: KindMap, Map: map[string]Value{
		"desired": {Kind: KindMap, Map: map[string]Value{"path": Scalar("/etc/hosts")}},
	}}
	got, ok := v.Lookup("desired.path")
	if !ok || got.Scalar != "/etc/hosts" {
		t.Errorf("Lookup = %q, %v", got.Scalar, ok)
	}
	if _, ok := v.Lookup("desired.missing"); ok {
		t.Error("Lookup found a field that is not there")
	}
	if _, ok := v.Lookup("desired.path.deeper"); ok {
		t.Error("Lookup walked through a scalar")
	}
}

func TestCloneDoesNotAlias(t *testing.T) {
	original := Value{Kind: KindMap, Map: map[string]Value{
		"list": {Kind: KindList, List: []Value{Scalar("one")}},
	}}
	copied := original.Clone()
	copied.Map["list"].List[0] = Scalar("two")

	got, _ := original.Lookup("list")
	if got.List[0].Scalar != "one" {
		t.Error("changing the clone changed the original")
	}
}

func TestValidName(t *testing.T) {
	good := []string{"nginx", "web-001", "ca-certificates", "net.ipv4.ip_forward", "g++"}
	for _, s := range good {
		if !ValidName(s) {
			t.Errorf("ValidName(%q) = false, want true", s)
		}
	}
	bad := []string{"", "nginx; curl", "with space", "sl/ash", "quote\"d", "new\nline"}
	for _, s := range bad {
		if ValidName(s) {
			t.Errorf("ValidName(%q) = true, want false", s)
		}
	}
}

func TestValidLabelKeyAllowsOneSlash(t *testing.T) {
	if !ValidLabelKey("datum/host") {
		t.Error("datum/host should be a valid label key")
	}
	if ValidLabelKey("a/b/c") {
		t.Error("two slashes should not be valid")
	}
}

func TestSafeTextRejectsBidiOverrides(t *testing.T) {
	if err := SafeText("plain text"); err != nil {
		t.Errorf("SafeText on plain text = %v", err)
	}
	if err := SafeText("harmless\u202eevil"); err == nil {
		t.Error("a text direction override should be rejected")
	}
	if err := SafeText("with\x00nul"); err == nil {
		t.Error("a NUL byte should be rejected")
	}
}

func TestParseReference(t *testing.T) {
	ref, ok := ParseReference("Package[nginx]")
	if !ok || ref.Type != "Package" || ref.Name != "nginx" {
		t.Errorf("ParseReference = %v, %v", ref, ok)
	}
	for _, s := range []string{"nginx", "Package[]", "[nginx]", "Widget[nginx]", "Package[nginx"} {
		if _, ok := ParseReference(s); ok {
			t.Errorf("ParseReference(%q) should have failed", s)
		}
	}
}
