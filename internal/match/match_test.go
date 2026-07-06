// SPDX-License-Identifier: Apache-2.0

package match

import (
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
)

var webHost = map[string]string{
	"datum/host":   "web-001",
	"environment":  "production",
	"site":         "london",
	"role":         "web",
	"architecture": "amd64",
}

func TestEmptyMatcherMatchesEveryHost(t *testing.T) {
	ok, reasons := Evaluate(document.Matcher{}, webHost)
	if !ok {
		t.Error("an empty matcher should match")
	}
	if len(reasons) != 0 {
		t.Errorf("reasons = %v, want none", reasons)
	}
}

func TestLabelsMustAllHold(t *testing.T) {
	m := document.Matcher{Labels: map[string]string{"role": "web", "site": "london"}}
	if ok, _ := Evaluate(m, webHost); !ok {
		t.Error("both labels hold, so it should match")
	}

	m = document.Matcher{Labels: map[string]string{"role": "web", "site": "frankfurt"}}
	if ok, _ := Evaluate(m, webHost); ok {
		t.Error("one label does not hold, so it should not match")
	}
}

func TestMissingLabelDoesNotMatch(t *testing.T) {
	m := document.Matcher{Labels: map[string]string{"tier": "gold"}}
	if ok, _ := Evaluate(m, webHost); ok {
		t.Error("a label the host does not declare should not match")
	}
}

func TestOneOf(t *testing.T) {
	m := document.Matcher{OneOf: map[string][]string{"site": {"london", "frankfurt"}}}
	if ok, _ := Evaluate(m, webHost); !ok {
		t.Error("london is listed, so it should match")
	}

	m = document.Matcher{OneOf: map[string][]string{"site": {"berlin"}}}
	if ok, _ := Evaluate(m, webHost); ok {
		t.Error("london is not listed, so it should not match")
	}
}

// noneOf holds when the label is absent as well as when it has a value that is
// not listed, which is what makes it usable for excluding a subset.
func TestNoneOf(t *testing.T) {
	m := document.Matcher{NoneOf: map[string][]string{"tier": {"legacy"}}}
	if ok, _ := Evaluate(m, webHost); !ok {
		t.Error("the host has no tier label, so noneOf should hold")
	}

	legacy := map[string]string{"tier": "legacy"}
	if ok, _ := Evaluate(m, legacy); ok {
		t.Error("tier is legacy, so noneOf should not hold")
	}

	modern := map[string]string{"tier": "current"}
	if ok, _ := Evaluate(m, modern); !ok {
		t.Error("tier is not legacy, so noneOf should hold")
	}
}

func TestHasAndMissing(t *testing.T) {
	if ok, _ := Evaluate(document.Matcher{Has: []string{"role"}}, webHost); !ok {
		t.Error("role is present, so has should hold")
	}
	if ok, _ := Evaluate(document.Matcher{Has: []string{"tier"}}, webHost); ok {
		t.Error("tier is absent, so has should not hold")
	}
	if ok, _ := Evaluate(document.Matcher{Missing: []string{"tier"}}, webHost); !ok {
		t.Error("tier is absent, so missing should hold")
	}
	if ok, _ := Evaluate(document.Matcher{Missing: []string{"role"}}, webHost); ok {
		t.Error("role is present, so missing should not hold")
	}
}

func TestFormsAreCombinedAsAConjunction(t *testing.T) {
	m := document.Matcher{
		Labels:  map[string]string{"role": "web"},
		OneOf:   map[string][]string{"site": {"london"}},
		NoneOf:  map[string][]string{"tier": {"legacy"}},
		Has:     []string{"architecture"},
		Missing: []string{"decommissioned"},
	}
	if ok, _ := Evaluate(m, webHost); !ok {
		t.Error("every form holds, so it should match")
	}

	// Break one form and the whole matcher fails.
	m.Missing = []string{"role"}
	if ok, _ := Evaluate(m, webHost); ok {
		t.Error("one failing form should fail the matcher")
	}
}

func TestReasonsAreReportedAndStable(t *testing.T) {
	m := document.Matcher{
		Labels: map[string]string{"role": "web", "environment": "production"},
		OneOf:  map[string][]string{"site": {"london"}},
		Has:    []string{"architecture"},
	}
	_, first := Evaluate(m, webHost)
	_, second := Evaluate(m, webHost)

	if first.String() != second.String() {
		t.Errorf("reasons are not stable: %q then %q", first, second)
	}
	want := "environment=production, role=web, site=london, architecture=amd64"
	if first.String() != want {
		t.Errorf("reasons = %q, want %q", first, want)
	}
}

// noneOf and missing hold by absence, so there is no label to report as a reason.
func TestNegativeFormsProduceNoReasons(t *testing.T) {
	m := document.Matcher{
		NoneOf:  map[string][]string{"tier": {"legacy"}},
		Missing: []string{"decommissioned"},
	}
	ok, reasons := Evaluate(m, webHost)
	if !ok {
		t.Fatal("should match")
	}
	if len(reasons) != 0 {
		t.Errorf("reasons = %v, want none", reasons)
	}
}

func TestMatchHostLabel(t *testing.T) {
	m := document.Matcher{Labels: map[string]string{document.HostLabel: "web-001"}}
	if ok, reasons := Evaluate(m, webHost); !ok || reasons.String() != "datum/host=web-001" {
		t.Errorf("ok=%v reasons=%q", ok, reasons)
	}
}

func TestValidateRejectsContradictions(t *testing.T) {
	cases := []struct {
		name string
		m    document.Matcher
		want string
	}{
		{
			name: "has and missing",
			m:    document.Matcher{Has: []string{"role"}, Missing: []string{"role"}},
			want: "both present and absent",
		},
		{
			name: "labels and missing",
			m:    document.Matcher{Labels: map[string]string{"role": "web"}, Missing: []string{"role"}},
			want: "to have a value and to be absent",
		},
		{
			name: "empty oneOf",
			m:    document.Matcher{OneOf: map[string][]string{"site": {}}},
			want: "can never match",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var errs document.Errors
			Validate(c.m, document.Position{File: "fleet/layer.yaml"}, &errs)
			if errs.Len() == 0 {
				t.Fatalf("expected an error mentioning %q", c.want)
			}
			if got := errs.Error(); !strings.Contains(got, c.want) {
				t.Errorf("error = %q, want it to mention %q", got, c.want)
			}
		})
	}
}

func TestDescribe(t *testing.T) {
	if got := Describe(document.Matcher{}); got != "every host" {
		t.Errorf("Describe(empty) = %q", got)
	}
	m := document.Matcher{
		Labels: map[string]string{"role": "web"},
		OneOf:  map[string][]string{"site": {"london", "frankfurt"}},
	}
	want := "role=web, site in [london frankfurt]"
	if got := Describe(m); got != want {
		t.Errorf("Describe = %q, want %q", got, want)
	}
}
