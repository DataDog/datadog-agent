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
	"regexp"
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

// Agent contains the existing common Agent fields. Examples are shared here,
// rather than repeated in each environment template. Installer-specific semantic
// rules are still checked by the selected installer.
type Agent struct {
	Install      string            `yaml:"install" config:"required" description:"Agent installation method selected for this environment."`
	Version      string            `yaml:"version,omitempty" example:"7.69.0" description:"Released Agent version to test; for local images use image instead."`
	Image        string            `yaml:"image,omitempty" description:"Existing local development image, with a semver-shaped tag."`
	APIKey       string            `yaml:"api-key,omitempty" config:"secret"`
	Config       string            `yaml:"config,omitempty" description:"Additional datadog.yaml configuration."`
	Integrations map[string]string `yaml:"integrations,omitempty"`
}

var agentSchema = configschema.Must[Agent]()

func (f *File) FakeIntakeEnabled() bool { return f.Environment.Fixtures.FakeIntake }

var (
	versionRegexp      = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	integrationPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+\.d$`)
	imageRefRegexp     = regexp.MustCompile(`^[a-zA-Z0-9.-]+(:[0-9]+)?/[a-zA-Z0-9/._-]+:[a-zA-Z0-9._-]+$`)
	semverTagRegexp    = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9._-]+)?$`)
)

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
	f.Agent, _, err = agentSchema.DecodeNode(members["agent"], "agent")
	if err != nil {
		errs = append(errs, err)
	} else {
		errs = append(errs, f.validateAgent()...)
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

func (f *File) validateAgent() []error {
	var errs []error
	a := &f.Agent
	if a.Install == "" {
		errs = append(errs, errf("agent.install", "missing"))
	}
	if a.Version != "" && !versionRegexp.MatchString(a.Version) {
		errs = append(errs, errf("agent.version", "%q is not a released agent version (expected e.g. \"7.69.0\")", a.Version))
	}
	if a.Image != "" {
		if !imageRefRegexp.MatchString(a.Image) {
			errs = append(errs, errf("agent.image", "expected a fully-qualified image reference with tag"))
		} else if !semverTagRegexp.MatchString(a.Image[strings.LastIndex(a.Image, ":")+1:]) {
			errs = append(errs, errf("agent.image", "tag is not semver-shaped (expected e.g. \"7.99.0-e2ectl\")"))
		}
	}
	if a.Config != "" {
		var cfg map[string]any
		if err := yaml.Unmarshal([]byte(a.Config), &cfg); err != nil {
			errs = append(errs, errf("agent.config", "not valid YAML"))
		}
	}
	for folder := range a.Integrations {
		if !integrationPattern.MatchString(folder) {
			errs = append(errs, errf("agent.integrations", "%q is not a valid conf.d folder name", folder))
		}
	}
	return errs
}

// Example composes the generated environment schema with the shared fixture and
// Agent schemas. Only envelope names/selectors are specified here, not provider
// fields. Secret-store contents and live environment state are never consulted.
func Example(base, description, install string, section *yaml.Node) ([]byte, error) {
	fixtureNode, err := fixtures.Schema.Example(nil)
	if err != nil {
		return nil, err
	}
	agent, err := agentSchema.Example(map[string]any{"install": install})
	if err != nil {
		return nil, err
	}
	str := func(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }
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
