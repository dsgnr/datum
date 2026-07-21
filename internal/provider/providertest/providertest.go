// SPDX-License-Identifier: Apache-2.0

// Package providertest is an in-memory provider for testing the engine. A test
// using it needs no host, no root and no particular operating system.
package providertest

import (
	"context"
	"fmt"
	"sync"

	"github.com/dsgnr/datum/internal/document"
	"github.com/dsgnr/datum/internal/provider"
	"github.com/dsgnr/datum/internal/state"
)

// Target is the state of one thing on the pretend host.
type Target struct {
	Exists bool
	// Found describes something of the wrong kind here.
	Found  string
	Fields map[string]string
	// Unobservable fields this target cannot report.
	Unobservable []string
}

// Provider is a fake host, keyed by type and target identity.
type Provider struct {
	mu      sync.Mutex
	name    string
	types   []string
	targets map[string]Target
	// inert targets accept an apply and do not change.
	inert map[string]bool

	// ObserveErr and ApplyErr force a failure for one target.
	ObserveErr map[string]error
	ApplyErr   map[string]error

	// Applied records the actions in order.
	Applied []Call
}

// Call is one action the provider was asked to perform.
type Call struct {
	Ref    document.Reference
	Action state.Action
}

func New(name string, types ...string) *Provider {
	return &Provider{
		name:       name,
		types:      types,
		targets:    map[string]Target{},
		inert:      map[string]bool{},
		ObserveErr: map[string]error{},
		ApplyErr:   map[string]error{},
	}
}

func (p *Provider) Name() string    { return p.name }
func (p *Provider) Types() []string { return p.types }

func key(typeName, target string) string { return typeName + " " + target }

// Set puts a target on the host.
func (p *Provider) Set(typeName, target string, t Target) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.targets[key(typeName, target)] = t
}

// SetFields is the common case of a target that exists, with field values.
func (p *Provider) SetFields(typeName, target string, fields map[string]string) {
	p.Set(typeName, target, Target{Exists: true, Fields: fields})
}

// Unchanging makes Apply report success and leave the target alone, which is the
// provider bug verification exists to catch.
func (p *Provider) Unchanging(typeName, target string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inert[key(typeName, target)] = true
}

// Get reads a target back, to check that Apply did something.
func (p *Provider) Get(typeName, target string) (Target, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	t, ok := p.targets[key(typeName, target)]
	return t, ok
}

func (p *Provider) Observe(ctx context.Context, req provider.Request) (provider.Observation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	k := key(req.Ref.Type, req.Target)
	if err := p.ObserveErr[k]; err != nil {
		return provider.Observation{}, err
	}

	t, ok := p.targets[k]
	if !ok {
		return provider.Observation{Exists: false}, nil
	}

	out := provider.Observation{
		Exists:       t.Exists,
		Found:        t.Found,
		Fields:       map[string]document.Value{},
		Unobservable: t.Unobservable,
	}
	for name, value := range t.Fields {
		out.Fields[name] = document.Scalar(value)
	}
	return out, nil
}

// Apply changes the host, so observing again afterwards sees the effect.
func (p *Provider) Apply(ctx context.Context, req provider.Request, action state.Action) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	k := key(req.Ref.Type, req.Target)
	if err := p.ApplyErr[k]; err != nil {
		return err
	}
	p.Applied = append(p.Applied, Call{Ref: req.Ref, Action: action})
	if p.inert[k] {
		return nil
	}

	switch action {
	case state.Remove:
		delete(p.targets, k)
		return nil

	case state.Create, state.Update:
		t := p.targets[k]
		if t.Fields == nil {
			t.Fields = map[string]string{}
		}
		t.Exists = true
		t.Found = ""
		// Declared fields only, as a real provider does.
		for _, field := range req.Desired.Keys() {
			value := req.Desired.Map[field]
			if value.Kind != document.KindScalar {
				continue
			}
			t.Fields[field] = value.Scalar
		}
		p.targets[k] = t
		return nil

	case state.None, state.Skip:
		return nil
	}
	return fmt.Errorf("providertest: unexpected action %s", action)
}

// Order returns what was applied, for asserting plan order.
func (p *Provider) Order() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.Applied))
	for _, call := range p.Applied {
		out = append(out, call.Ref.String())
	}
	return out
}
