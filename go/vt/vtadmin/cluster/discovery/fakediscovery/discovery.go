/*
Copyright 2020 The Vitess Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package fakediscovery provides a fake, in-memory discovery implementation.
package fakediscovery

import (
	"context"
	"math/rand"

	"github.com/stretchr/testify/assert"

	"vitess.io/vitess/go/vt/vtadmin/cluster/discovery"

	vtadminpb "vitess.io/vitess/go/vt/proto/vtadmin"
)

type gates struct {
	byTag     map[string][]*vtadminpb.VTGate
	byName    map[string]*vtadminpb.VTGate
	shouldErr bool
}

// Fake is a fake discovery implementation for use in testing.
type Fake struct {
	gates *gates
}

// New returns a new fake.
func New() *Fake {
	return &Fake{
		gates: &gates{
			byTag:  map[string][]*vtadminpb.VTGate{},
			byName: map[string]*vtadminpb.VTGate{},
		},
	}
}

// AddTaggedGates adds the given gates to the discovery fake, associating each
// gate with each tag. To tag different gates with multiple tags, call multiple
// times with the same gates but different tag slices. Gates an uniquely
// identified by hostname.
func (d *Fake) AddTaggedGates(tags []string, gates ...*vtadminpb.VTGate) {
	for _, tag := range tags {
		d.gates.byTag[tag] = append(d.gates.byTag[tag], gates...)
	}

	for _, g := range gates {
		d.gates.byName[g.Hostname] = g
	}
}

// SetGatesError instructs whether the fake should return an error on gate
// discovery functions.
func (d *Fake) SetGatesError(shouldErr bool) {
	d.gates.shouldErr = shouldErr
}

var _ discovery.Discovery = (*Fake)(nil)

// DiscoverVTGates is part of the discovery.Discovery interface.
func (d *Fake) DiscoverVTGates(ctx context.Context, tags []string) ([]*vtadminpb.VTGate, error) {
	if d.gates.shouldErr {
		return nil, assert.AnError
	}

	if len(tags) == 0 {
		results := make([]*vtadminpb.VTGate, 0, len(d.gates.byName))
		for _, gate := range d.gates.byName {
			results = append(results, gate)
		}

		return results, nil
	}

	set := d.gates.byName

	for _, tag := range tags {
		intermediate := map[string]*vtadminpb.VTGate{}

		gates, ok := d.gates.byTag[tag]
		if !ok {
			return []*vtadminpb.VTGate{}, nil
		}

		for _, g := range gates {
			if _, ok := set[g.Hostname]; ok {
				intermediate[g.Hostname] = g
			}
		}

		set = intermediate
	}

	results := make([]*vtadminpb.VTGate, 0, len(set))

	for _, gate := range set {
		results = append(results, gate)
	}

	return results, nil
}

// DiscoverVTGate is part of the discovery.Discovery interface.
func (d *Fake) DiscoverVTGate(ctx context.Context, tags []string) (*vtadminpb.VTGate, error) {
	gates, err := d.DiscoverVTGates(ctx, tags)
	if err != nil {
		return nil, err
	}

	if len(gates) == 0 {
		return nil, assert.AnError
	}

	return gates[rand.Intn(len(gates))], nil
}

// DiscoverVTGateAddr is part of the discovery.Discovery interface.
func (d *Fake) DiscoverVTGateAddr(ctx context.Context, tags []string) (string, error) {
	gate, err := d.DiscoverVTGate(ctx, tags)
	if err != nil {
		return "", err
	}

	return gate.Hostname, nil
}
