// SPDX-License-Identifier: Apache-2.0

package document

import "sort"

// Domain groups resource types by the part of a host they describe.
//
// A domain is a grouping and nothing more. It is not part of a resource's identity, it
// does not appear in a document, and no reconciliation behaviour depends on it. The
// grouping keeps the type documentation navigable and gives later work a place to hang
// per-domain behaviour without inventing the taxonomy at the same time.
type Domain string

const (
	// DomainCore covers packages, package sources and the filesystem.
	DomainCore Domain = "Core"

	// DomainIdentity covers the local account database.
	DomainIdentity Domain = "Identity"

	// DomainRuntime covers what is running on the host.
	DomainRuntime Domain = "Runtime"

	// DomainKernel covers kernel parameters.
	DomainKernel Domain = "Kernel"
)

// Domains is every domain, in the order the documentation presents them. Core comes
// first because most manifests are mostly Core resources.
var Domains = []Domain{DomainCore, DomainIdentity, DomainRuntime, DomainKernel}

// domainOf places every type in ResourceTypes in exactly one domain. A type missing from
// here is a bug rather than a repository problem, which TestEveryResourceTypeHasADomain
// checks.
var domainOf = map[string]Domain{
	"Package":    DomainCore,
	"Repository": DomainCore,
	"File":       DomainCore,
	"Directory":  DomainCore,
	"Symlink":    DomainCore,

	"User":  DomainIdentity,
	"Group": DomainIdentity,

	"Service": DomainRuntime,

	"Sysctl": DomainKernel,
}

// DomainOf returns the domain a resource type belongs to, and false for a type that is
// not recognised.
func DomainOf(typeName string) (Domain, bool) {
	domain, ok := domainOf[typeName]
	return domain, ok
}

// TypesIn returns the types in one domain, sorted by name so callers get a stable order.
func TypesIn(domain Domain) []string {
	var names []string
	for typeName, d := range domainOf {
		if d == domain {
			names = append(names, typeName)
		}
	}
	sort.Strings(names)
	return names
}
