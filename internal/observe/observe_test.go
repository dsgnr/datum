// SPDX-License-Identifier: Apache-2.0

package observe

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/graph"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/provider/providertest"
	"github.com/dsgnr/datum/internal/resolve"
)

func resource(typeName, name string, fields map[string]string) resolve.Resource {
	desired := document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}
	for k, v := range fields {
		desired.Map[k] = document.Scalar(v)
	}
	return resolve.Resource{
		Ref:     document.Reference{Type: typeName, Name: name},
		Desired: desired,
	}
}

func observe(t *testing.T, providers provider.Set, resources ...resolve.Resource) State {
	t.Helper()
	m := resolve.Manifest{Host: "web-001", Resources: resources}
	g, err := graph.Build(m)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	got, err := Host(context.Background(), g, providers, "/repo")
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	return got
}

func TestObserveReadsEveryResource(t *testing.T) {
	host := providertest.New("fake", "Package")
	host.SetFields("Package", "curl", map[string]string{"version": "8.5.0"})

	got := observe(t, provider.NewSet(host),
		resource("Package", "curl", map[string]string{"state": "present"}),
		resource("Package", "git", map[string]string{"state": "present"}),
	)

	if len(got.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(got.Results))
	}

	curl, ok := got.For(document.Reference{Type: "Package", Name: "curl"})
	if !ok {
		t.Fatal("curl is missing")
	}
	if !curl.Observation.Exists {
		t.Error("curl should exist")
	}
	if version, _ := curl.Observation.Value("version"); version.Scalar != "8.5.0" {
		t.Errorf("version = %q", version.Scalar)
	}

	git, _ := got.For(document.Reference{Type: "Package", Name: "git"})
	if git.Observation.Exists {
		t.Error("git should not exist")
	}
}

func TestObserveSkipsTypesWithNoProvider(t *testing.T) {
	host := providertest.New("fake", "Package")

	got := observe(t, provider.NewSet(host),
		resource("Package", "curl", map[string]string{"state": "present"}),
		resource("Service", "nginx", map[string]string{"state": "running", "enabled": "true"}),
		resource("Sysctl", "net.ipv4.ip_forward", map[string]string{"value": "1"}),
	)

	if want := "Service,Sysctl"; strings.Join(got.Unsupported(), ",") != want {
		t.Errorf("Unsupported = %v, want %s", got.Unsupported(), want)
	}

	service, _ := got.For(document.Reference{Type: "Service", Name: "nginx"})
	if !service.Skipped {
		t.Error("the service should be skipped")
	}
	if service.Provider != "" {
		t.Errorf("a skipped resource has no provider, got %q", service.Provider)
	}
}

func TestObserveRecordsAReadFailure(t *testing.T) {
	host := providertest.New("fake", "File")
	host.ObserveErr["File /etc/shadow"] = errors.New("permission denied")

	got := observe(t, provider.NewSet(host),
		resource("File", "shadow", map[string]string{"path": "/etc/shadow"}),
	)

	result, _ := got.For(document.Reference{Type: "File", Name: "shadow"})
	if result.Err == nil {
		t.Fatal("the failure should be recorded rather than swallowed")
	}
	if result.Skipped {
		t.Error("a read failure is not the same as an unsupported type")
	}
}

// Observation follows plan order so that a report and a plan line up when read
// side by side.
func TestObserveFollowsGraphOrder(t *testing.T) {
	host := providertest.New("fake", "Package", "File")

	file := resource("File", "nginx-config", map[string]string{"path": "/etc/nginx/nginx.conf"})
	file.Requires = []document.Reference{{Type: "Package", Name: "nginx"}}

	got := observe(t, provider.NewSet(host), file, resource("Package", "nginx", map[string]string{"state": "present"}))

	if got.Results[0].Ref.String() != "Package[nginx]" {
		t.Errorf("first observed = %s, want Package[nginx]", got.Results[0].Ref)
	}
}

// Only declared targets are read. A host with thousands of packages and three
// declared ones does three reads.
func TestObserveDoesNotInventoryTheHost(t *testing.T) {
	host := providertest.New("fake", "Package")
	for _, name := range []string{"curl", "git", "vim", "unrelated-1", "unrelated-2"} {
		host.SetFields("Package", name, nil)
	}

	got := observe(t, provider.NewSet(host),
		resource("Package", "curl", map[string]string{"state": "present"}),
	)
	if len(got.Results) != 1 {
		t.Errorf("got %d results, want 1", len(got.Results))
	}
}

func TestObserveStopsOnACancelledContext(t *testing.T) {
	host := providertest.New("fake", "Package")
	m := resolve.Manifest{Host: "web-001", Resources: []resolve.Resource{
		resource("Package", "curl", map[string]string{"state": "present"}),
	}}
	g, err := graph.Build(m)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Host(ctx, g, provider.NewSet(host), "/repo"); err == nil {
		t.Error("a cancelled context should stop the pass")
	}
}
