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
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/fixtures"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/workloads"
	cat "github.com/DataDog/datadog-agent/test/e2e-framework/testing/workloads/catalog"
	"go.yaml.in/yaml/v3"
)

const SchemaVersion = 1

type File struct {
	Schema      int
	Environment Environment
	Agent       Agent
	Workloads   workloads.Config
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

// Agent is the simplified installation surface: the user picks at most one
// source (source/pipeline/version) plus common fields; the install mechanism
// and artifact provider are DERIVED from (source, environment base) and held
// in the internal fields below. The installer-owned sections still exist,
// but only the derivation writes them — agent.install and agent.build are no
// longer user-facing YAML (breaking change; old shapes are rejected with a
// pointer to the new one).
type ReceiverSelection struct {
	Type        string
	Section     []byte
	SectionNode *yaml.Node
}

type BuildSelection struct {
	Provider    string
	Section     []byte
	SectionNode *yaml.Node
}

type Agent struct {
	// Selection is the parsed user-facing agent input.
	Selection agent.Config
	Receiver  *ReceiverSelection

	// Install, Section and Build are the derived internal selection consumed
	// by the existing installer and build-provider machinery. SectionNode is
	// nil: the section is generated, so Decode (not DecodeNode) applies.
	Install string
	Section []byte
	Build   *BuildSelection
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
		if key != "schema" && key != "environment" && key != "agent" && key != "workloads" {
			errs = append(errs, errf(key, "unknown top-level field (supported: schema, environment, agent, workloads)"))
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
	// Workloads is a list, not a mapping — decode each item through the
	// schema for shape validation, then validate form compatibility at
	// prepare time.
	f.Workloads.Workloads, err = decodeWorkloadList(members["workloads"])
	if err != nil {
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

// parseAgent parses the simplified agent surface: at most one source
// (source/pipeline/version) plus the common config/integrations/values fields
// and the receiver selection. The legacy install/build selectors and the
// installer-owned sections are rejected with a pointer to the new shape — a
// deliberate breaking change, no dual format. The internal installer and
// build-provider selections are then DERIVED from (base, selection) so the
// existing machinery consumes them unchanged.
func (f *File) parseAgent(n *yaml.Node) error {
	if n == nil {
		return errf("agent", "missing")
	}
	// Mapping validates the envelope's key shape (string keys, no duplicates).
	if _, err := configschema.Mapping(n, "agent"); err != nil {
		return err
	}
	// Iterate original input order for diagnostics, independently of the
	// fields' positions.
	var err error
	filtered := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i < len(n.Content); i += 2 {
		key, value := n.Content[i], n.Content[i+1]
		switch key.Value {
		case "source", "pipeline", "version", "config", "integrations", "values":
			filtered.Content = append(filtered.Content, key, value)
		case "receiver":
			f.Agent.Receiver, err = parseReceiver(value)
			if err != nil {
				return err
			}
		case "install":
			return errf("agent.install", "no longer exists: the install mechanism is derived from the agent source (source, pipeline or version) and environment.base — see the e2ectl README")
		case "build":
			return errf("agent.build", "no longer exists: the artifact source is derived from the agent source — see the e2ectl README")
		case "binary", "helm", "script", "package":
			return errf("agent."+key.Value, "installer sections are no longer user-facing: set the common agent fields (config, integrations, values) and let the source select the mechanism — see the e2ectl README")
		default:
			return errf("agent."+key.Value, "unknown field (supported: source, pipeline, version, config, integrations, values, receiver)")
		}
	}
	selection, _, err := agent.Schema.DecodeNode(filtered, "agent")
	if err != nil {
		return err
	}
	f.Agent.Selection = selection
	resolved, err := agent.Derive(f.Environment.Base, selection)
	if err != nil {
		return err
	}
	f.Agent.Install = resolved.Installer
	f.Agent.Section = resolved.Section
	if resolved.Build != nil {
		f.Agent.Build = &BuildSelection{Provider: resolved.Build.Provider, Section: resolved.Build.Section}
	}
	return nil
}

// Example composes the generated environment schema, the shared fixture schema
// and the simplified agent section (its default source selection plus the
// optional receiver). Only envelope names are specified here, not installer
// fields. Secret-store contents and live environment state are never consulted.
func Example(base, description string, section, agentSection *yaml.Node, receiverSection ...*yaml.Node) ([]byte, error) {
	fixtureNode, err := fixtures.Schema.Example(nil)
	if err != nil {
		return nil, err
	}
	str := func(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }
	agentNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if agentSection != nil {
		agentNode.Content = append(agentNode.Content, agentSection.Content...)
	}
	if len(receiverSection) > 0 && receiverSection[0] != nil {
		agentNode.Content = append(agentNode.Content, str("receiver"), receiverSection[0])
	}
	env := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{str("base"), str(base)}}
	env.Content = append(env.Content, fixtureNode.Content...)
	env.Content = append(env.Content, str(base), section)
	workloadsNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "workloads"}
	workloadsNode.HeadComment = strings.Join([]string{
		"workloads: test apps deployed into the environment (optional). Three forms:",
		"  - app: a named workload from the framework catalog —",
		"    " + strings.Join(cat.Names(), ", "),
		"  - manifest: inline Kubernetes/Docker-compose YAML or a path to a file",
		"  - image: a Docker image reference (container-native bases)",
		"Example:",
		"  workloads:",
		"    - app: nginx",
		"    - app: dogstatsd",
		"    - manifest: |",
		"        apiVersion: v1",
		"        kind: Pod",
		"        metadata:",
		"          name: my-pod",
		"        spec:",
		"          containers:",
		"            - name: app",
		"              image: busybox",
		"              command: [\"sleep\", \"3600\"]",
	}, "\n")
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", HeadComment: description + "\nGenerated starter config: review example values before provisioning.\nCredentials come from the runner profile, not this file.", Content: []*yaml.Node{
		str("schema"), {Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(SchemaVersion)},
		str("environment"), env,
		str("agent"), agentNode,
		workloadsNode, {Kind: yaml.ScalarNode, Tag: "!!null", Value: ""},
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

// decodeWorkloadList decodes the workloads section (a YAML sequence) into
// typed Workload entries with schema validation. A null node (an empty
// `workloads:` key) is treated as absent.
func decodeWorkloadList(n *yaml.Node) ([]workloads.Workload, error) {
	if n == nil {
		return nil, nil
	}
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
		return nil, nil
	}
	if n.Kind != yaml.SequenceNode {
		return nil, errf("workloads", "expected a list of workloads")
	}
	var result []workloads.Workload
	for i, item := range n.Content {
		w, _, err := workloads.Schema.DecodeNode(item, fmt.Sprintf("workloads[%d]", i))
		if err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, nil
}

// Receiver contents are decoded by the explicit receiver registry, not by the
// infrastructure parser. This mirrors the installer-owned section contract.
func parseReceiver(n *yaml.Node) (*ReceiverSelection, error) {
	members, err := configschema.Mapping(n, "agent.receiver")
	if err != nil {
		return nil, err
	}
	typ := members["type"]
	if typ == nil || typ.Tag != "!!str" || typ.Value == "" {
		return nil, errf("agent.receiver.type", "expected a nonempty string")
	}
	r := &ReceiverSelection{Type: typ.Value}
	for key, value := range members {
		if key == "type" {
			continue
		}
		if key != r.Type {
			return nil, errf("agent.receiver."+key, "section does not match type %q", r.Type)
		}
		r.SectionNode = value
		r.Section, err = configschema.Encode(value)
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}
