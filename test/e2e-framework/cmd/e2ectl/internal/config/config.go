// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config parses the common e2ectl envelope. Environment-specific fields
// are prepared by the schema associated with the selected driver registration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/fixtures"
	"go.yaml.in/yaml/v3"
)

const SchemaVersion = 1

type File struct {
	Schema      int
	Environment Environment
	Agent       Agent
	Path        string
	source      []byte
}

// Source returns the input that was parsed, not a later re-read of a file an
// operator may edit while a long-running install or update is in progress.
func (f *File) Source() []byte { return bytes.Clone(f.source) }

// Environment is the common envelope, not a union of provider parameters.
type Environment struct {
	Base        string
	Fixtures    fixtures.Config
	Section     []byte
	SectionNode *yaml.Node // original positions for schema diagnostics
}

// Agent mirrors Environment: the installer selector plus the installer-owned
// section, held as a node until the selected installer's schema consumes it.
// There is deliberately no shared agent field bag: what an install consumes
// (version, image, config, integrations) is defined by the installer's own
// section schema, so fields irrelevant to an installation method cannot be
// written at all.
type Agent struct {
	Install     string
	Section     []byte
	SectionNode *yaml.Node // original positions for schema diagnostics
}

func (f *File) FakeIntakeEnabled() bool { return f.Environment.Fixtures.FakeIntake }

func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	f, errs := Parse(data)
	if err := NewErrors(errs); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f.Path = path
	return f, nil
}

func Parse(data []byte) (*File, []error) {
	root, err := configschema.ParseDocument(data)
	if err != nil {
		return nil, []error{err}
	}
	members, err := configschema.Mapping(root, "config")
	if err != nil {
		return nil, []error{err}
	}
	f := &File{}
	var errs []error
	for key := range members {
		if key != "schema" && key != "environment" && key != "agent" {
			errs = append(errs, errf(key, "unknown top-level field (supported: schema, environment, agent)"))
		}
	}
	if n := members["schema"]; n != nil {
		if n.Tag != "!!int" || n.Decode(&f.Schema) != nil {
			errs = append(errs, errf("schema", "expected an integer"))
		}
	}
	if f.Schema != SchemaVersion {
		errs = append(errs, errf("schema", "must be %d", SchemaVersion))
	}
	if err := f.parseEnvironment(members["environment"]); err != nil {
		errs = append(errs, err)
	}
	if err := f.parseAgent(members["agent"]); err != nil {
		errs = append(errs, err)
	}
	if len(errs) != 0 {
		return nil, errs
	}
	f.source = bytes.Clone(data)
	return f, nil
}

func (f *File) parseEnvironment(n *yaml.Node) error {
	if n == nil {
		return errf("environment.base", "missing")
	}
	members, err := configschema.Mapping(n, "environment")
	if err != nil {
		return err
	}
	base := members["base"]
	if base == nil || base.Tag != "!!str" || base.Value == "" {
		return errf("environment.base", "expected a nonempty string")
	}
	f.Environment.Base = base.Value
	fixtureNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	// Iterate original input order for diagnostics, independently of base's position.
	for i := 0; i < len(n.Content); i += 2 {
		key, value := n.Content[i], n.Content[i+1]
		switch key.Value {
		case "base":
		case "fakeintake":
			fixtureNode.Content = append(fixtureNode.Content, key, value)
		default:
			if key.Value != base.Value {
				return errf("environment."+key.Value, "not supported for base %q (only the %q section is)", base.Value, base.Value)
			}
			f.Environment.SectionNode = value
			f.Environment.Section, err = configschema.Encode(value)
			if err != nil {
				return err
			}
		}
	}
	f.Environment.Fixtures, _, err = fixtures.Schema.DecodeNode(fixtureNode, "environment")
	return err
}

// parseAgent mirrors parseEnvironment: the `install` selector plus exactly one
// installer-owned section named by it. Section *contents* are validated later,
// when the selected installer's schema consumes them — the same two-stage flow
// as the environment section (base selects a driver, the driver validates).
func (f *File) parseAgent(n *yaml.Node) error {
	if n == nil {
		return errf("agent.install", "missing")
	}
	members, err := configschema.Mapping(n, "agent")
	if err != nil {
		return err
	}
	inst := members["install"]
	if inst == nil || inst.Tag != "!!str" || inst.Value == "" {
		return errf("agent.install", "expected a nonempty string")
	}
	f.Agent.Install = inst.Value
	// Iterate original input order for diagnostics, independently of install's position.
	for i := 0; i < len(n.Content); i += 2 {
		key, value := n.Content[i], n.Content[i+1]
		switch key.Value {
		case "install":
		case f.Agent.Install:
			f.Agent.SectionNode = value
			f.Agent.Section, err = configschema.Encode(value)
			if err != nil {
				return err
			}
		default:
			return errf("agent."+key.Value, "unknown field (agent sections are installer-owned; set it under agent.%s)", f.Agent.Install)
		}
	}
	return nil
}

// Example composes the generated environment schema, the shared fixture schema
// and the selected installer's agent section. Only envelope names/selectors are
// specified here, not provider or installer fields. Secret-store contents and
// live environment state are never consulted.
func Example(base, description, install string, section, agentSection *yaml.Node) ([]byte, error) {
	fixtureNode, err := fixtures.Schema.Example(nil)
	if err != nil {
		return nil, err
	}
	str := func(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }
	installKey := str("install")
	installKey.HeadComment = "Agent installation method selected for this environment."
	agent := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{installKey, str(install)}}
	if agentSection != nil {
		agent.Content = append(agent.Content, str(install), agentSection)
	}
	env := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{str("base"), str(base)}}
	env.Content = append(env.Content, fixtureNode.Content...)
	env.Content = append(env.Content, str(base), section)
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", HeadComment: description + "\nGenerated starter config: review example values before provisioning.\nCredentials come from the runner profile, not this file.", Content: []*yaml.Node{
		str("schema"), {Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(SchemaVersion)},
		str("environment"), env,
		str("agent"), agent,
	}}
	return configschema.Encode(root)
}

// StrictDecode remains for legacy callers; new typed registrations use Schema.
func StrictDecode(raw []byte, out any) error {
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	return dec.Decode(out)
}

func NewErrors(errs []error) error { return errors.Join(errs...) }
func errf(field, format string, args ...any) error {
	return fmt.Errorf("%s: %s", field, fmt.Sprintf(format, args...))
}
