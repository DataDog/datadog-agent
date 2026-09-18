// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package receiver owns the CLI's explicit, typed outbound receiver registry.
// Installers consume prepared public plans, never receiver-specific branches.
package receiver

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	bc "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/receiverconfig/blackhole"
	dc "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/receiverconfig/datadog"
	fc "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/receiverconfig/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers/blackhole"
)

// Facts are inventory projection, not a topology or an initialized environment.
type Facts struct {
	FakeIntake      *outputs.FakeintakeOutput
	SeparateNetwork bool
}

type Definition struct {
	ID          string
	Description string
	validate    func(*config.ReceiverSelection) error
	resolve     func(*config.ReceiverSelection, Facts) (receivers.Plan, error)
	Serve       func() http.Handler
}

// Define binds a type-owned schema to a pure resolver. Registration performs no
// secret lookup, network operation or environment initialization.
func Define[P any](id, description string, schema *configschema.Schema[P], resolve func(P, Facts) (receivers.Plan, error), serve func() http.Handler) Definition {
	if schema == nil || id == "" || description == "" {
		panic("receiver registration requires schema, ID and description")
	}
	decode := func(s *config.ReceiverSelection) (P, error) {
		if s.SectionNode != nil {
			p, _, err := schema.DecodeNode(s.SectionNode, "agent.receiver."+id)
			return p, err
		}
		p, _, err := schema.Decode(s.Section, "agent.receiver."+id)
		return p, err
	}
	return Definition{ID: id, Description: description, Serve: serve,
		validate: func(s *config.ReceiverSelection) error { _, err := decode(s); return err },
		resolve: func(s *config.ReceiverSelection, f Facts) (receivers.Plan, error) {
			p, err := decode(s)
			if err != nil {
				return receivers.Plan{}, err
			}
			return resolve(p, f)
		},
	}
}

var registry = []Definition{
	Define("fakeintake", "Environment capture fixture (forwarding remains fixture-owned)", fc.Schema, func(_ fc.Config, f Facts) (receivers.Plan, error) {
		if f.FakeIntake == nil {
			return receivers.Plan{}, fmt.Errorf("selected fakeintake fixture is missing; refusing native fallback")
		}
		if f.FakeIntake.AgentURL == "" {
			return receivers.Plan{}, fmt.Errorf("fakeintake snapshot has no AgentURL; recreate the environment to establish producer reachability")
		}
		endpoint, err := receivers.AgentURL(f.FakeIntake.AgentURL, f.SeparateNetwork)
		if err != nil {
			return receivers.Plan{}, err
		}
		p, err := receivers.Capture("fakeintake", endpoint, f.FakeIntake.QueryURL)
		p.Warnings = append(p.Warnings, "Fixture forwarding is unchanged and may forward to dddev in cloud environments.")
		return p, err
	}, nil),
	Define("datadog", "Native Datadog ingestion with explicit site and runner reference", dc.Schema, func(p dc.Config, _ Facts) (receivers.Plan, error) { return receivers.Datadog(p.Site, p.APIKeyRef) }, nil),
	Define("blackhole", "Stateless, externally managed HTTP intake sink", bc.Schema, func(p bc.Config, f Facts) (receivers.Plan, error) {
		endpoint, err := receivers.AgentURL(p.URL, f.SeparateNetwork)
		if err != nil {
			return receivers.Plan{}, err
		}
		return receivers.Capture("blackhole", endpoint, "")
	}, blackhole.Handler),
}

func Get(id string) (Definition, error) {
	for _, d := range registry {
		if d.ID == id {
			return d, nil
		}
	}
	return Definition{}, fmt.Errorf("unknown receiver type %q", id)
}
func Validate(s *config.ReceiverSelection) error {
	if s == nil {
		return nil
	}
	d, err := Get(s.Type)
	if err != nil {
		return err
	}
	return d.validate(s)
}
func Resolve(s *config.ReceiverSelection, f Facts) (*receivers.Plan, error) {
	if s == nil {
		return nil, nil
	}
	d, err := Get(s.Type)
	if err != nil {
		return nil, err
	}
	p, err := d.resolve(s, f)
	if err != nil {
		return nil, err
	}
	return &p, p.Validate()
}
