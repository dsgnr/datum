// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// examplePath is the fleet shipped in the repository. It is tested because an
// example that does not resolve is worse than no example.
const examplePath = "../../examples/fleet"

func TestExampleFleetValidates(t *testing.T) {
	got := invoke("validate", "-repo", filepath.FromSlash(examplePath))
	if got.code != exitOK {
		t.Fatalf("the shipped example does not validate:\n%s", got.all())
	}
	if !strings.Contains(got.out, "resolved 3 hosts, 0 errors") {
		t.Errorf("expected three hosts to resolve, got:\n%s", got.out)
	}
}

func TestExampleFleetHostsDifferAsDocumented(t *testing.T) {
	render := func(host string) string {
		t.Helper()
		got := invoke("render", "-host", host, "-repo", filepath.FromSlash(examplePath))
		if got.code != exitOK {
			t.Fatalf("render %s failed:\n%s", host, got.all())
		}
		return got.out
	}

	web := render("web-001")
	staging := render("web-002")
	db := render("db-001")

	// The web role reaches both web hosts and not the database host.
	for _, want := range []string{"Package[nginx]", "Service[nginx]"} {
		if !strings.Contains(web, want) {
			t.Errorf("web-001 should have %s", want)
		}
		if strings.Contains(db, want) {
			t.Errorf("db-001 should not have %s", want)
		}
	}

	// Production kernel tuning reaches production only.
	if !strings.Contains(web, "Sysctl[vm.swappiness]") {
		t.Error("web-001 is in production and should have vm.swappiness")
	}
	if strings.Contains(staging, "Sysctl[vm.swappiness]") {
		t.Error("web-002 is in staging and should not have vm.swappiness")
	}

	// The host override applies to one host.
	if !strings.Contains(web, "role-web, host-web-001") {
		t.Error("web-001 should show both layers contributing to the overridden file")
	}
}

func TestExampleFleetOverrideAndSubstitution(t *testing.T) {
	got := invoke("explain", "File[nginx-config]", "-host", "web-001", "-repo", filepath.FromSlash(examplePath))
	if got.code != exitOK {
		t.Fatalf("explain failed:\n%s", got.all())
	}
	if !strings.Contains(got.out, "overrides 0640 from role-web") {
		t.Errorf("expected the host override to be reported, got:\n%s", got.out)
	}

	got = invoke("explain", "File[motd]", "-host", "web-001", "-repo", filepath.FromSlash(examplePath))
	if got.code != exitOK {
		t.Fatalf("explain failed:\n%s", got.all())
	}
	for _, want := range []string{"substitutions", "labels.site", "london", "host", "web-001"} {
		if !strings.Contains(got.out, want) {
			t.Errorf("expected %q in the explanation, got:\n%s", want, got.out)
		}
	}
}
