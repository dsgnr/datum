// SPDX-License-Identifier: Apache-2.0

package document

import "testing"

// The domain table and the set of recognised types have to agree in both directions, or
// the documentation ends up listing a type no domain claims, or a domain claiming a type
// the parser rejects.
func TestEveryResourceTypeHasADomain(t *testing.T) {
	for typeName := range ResourceTypes {
		if _, ok := DomainOf(typeName); !ok {
			t.Errorf("%s is an accepted resource type in no domain", typeName)
		}
	}
	for typeName := range domainOf {
		if !ResourceTypes[typeName] {
			t.Errorf("%s is placed in a domain but is not an accepted resource type", typeName)
		}
	}
}

// A domain nobody is in would be a heading with nothing under it.
func TestEveryDomainHasTypes(t *testing.T) {
	for _, domain := range Domains {
		if len(TypesIn(domain)) == 0 {
			t.Errorf("domain %s has no types", domain)
		}
	}
}

// Domains is what the documentation orders itself by, so it has to hold every domain the
// table uses.
func TestDomainsListsEveryDomainInUse(t *testing.T) {
	listed := map[Domain]bool{}
	for _, domain := range Domains {
		if listed[domain] {
			t.Errorf("domain %s is listed twice", domain)
		}
		listed[domain] = true
	}
	for typeName, domain := range domainOf {
		if !listed[domain] {
			t.Errorf("%s is in domain %s, which Domains does not list", typeName, domain)
		}
	}
}

func TestTypesInIsSorted(t *testing.T) {
	got := TypesIn(DomainCore)
	want := []string{"Directory", "File", "Package", "Repository", "Symlink"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDomainOfRejectsAnUnknownType(t *testing.T) {
	if _, ok := DomainOf("Firewall"); ok {
		t.Error("Firewall is not a resource type and should be in no domain")
	}
}
