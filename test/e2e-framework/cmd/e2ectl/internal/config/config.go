// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package config defines and validates the e2ectl environment configuration
// (schema v1). Validation is deliberately strict: unknown fields, invalid
// types and invalid combinations all fail with field-anchored errors, before
// any environment is touched.
//
// The environment section is: common fields (base, fakeintake) plus ONE
// driver-owned section named after the base. The core validates what it owns;
// the driver strict-decodes its own section (config.StrictDecode), so
// unknown-field rejection works per driver and the core never grows
// per-environment types.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// SchemaVersion is the only config schema version supported so far.
const SchemaVersion = 1

// File is an e2ectl environment configuration file.
type File struct {
	Schema      int         `yaml:"schema"`
	Environment Environment `yaml:"environment"`
	Agent       Agent       `yaml:"agent"`

	// Path is the file this config was loaded from; set by Load, not part of
	// the YAML schema.
	Path string `yaml:"-"`
}

// Environment describes the environment to create: the common fields the
// core owns, plus the raw driver-owned section.
type Environment struct {
	Base string `yaml:"base"`
	// FakeIntake defaults to true. Pointer so "explicitly false" is representable.
	FakeIntake *bool `yaml:"fakeintake,omitempty"`
	// Section is the raw driver-owned section (the environment key matching
	// base), strict-decoded by the driver. Nil when absent.
	Section []byte `yaml:"-"`
}

// Agent describes how the Datadog agent is installed on the environment.
// The install method's own rules (version vs image requirements) live in
// the installers; the core checks only the generic shapes.
type Agent struct {
	Install      string            `yaml:"install"`
	Version      string            `yaml:"version,omitempty"`
	Image        string            `yaml:"image,omitempty"`
	APIKey       string            `yaml:"api-key,omitempty"`
	Config       string            `yaml:"config,omitempty"`
	Integrations map[string]string `yaml:"integrations,omitempty"`
}

// FakeIntakeEnabled reports whether the fakeintake should be deployed.
func (f *File) FakeIntakeEnabled() bool {
	if f.Environment.FakeIntake == nil {
		return true
	}
	return *f.Environment.FakeIntake
}

var (
	versionRegexp      = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	integrationPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+\.d$`)
	imageRefRegexp     = regexp.MustCompile(`^[a-zA-Z0-9.-]+(:[0-9]+)?/[a-zA-Z0-9/._-]+:[a-zA-Z0-9._-]+$`)
	semverTagRegexp    = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9._-]+)?$`)
)

// imageTag returns the tag part of an image reference (after the last ':').
func imageTag(ref string) string {
	for i := len(ref) - 1; i >= 0; i-- {
		if ref[i] == ':' {
			return ref[i+1:]
		}
	}
	return ""
}

// Load reads and validates the configuration at path. All validation errors
// are accumulated so the file can be fixed in a single pass.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	f, errs := Parse(data)
	if len(errs) > 0 {
		return nil, &Errors{errs: errs}
	}
	f.Path = path
	return f, nil
}

// Parse validates the raw content of a configuration file.
func Parse(data []byte) (*File, []error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, []error{fmt.Errorf("invalid YAML: %w", err)}
	}
	if len(doc.Content) == 0 {
		return nil, []error{errf("config", "empty file")}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, []error{errf("config", "expected a mapping at the top level")}
	}

	f := &File{}
	var errs []error
	var envNode, agentNode *yaml.Node

	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		switch k.Value {
		case "schema":
			var s int
			if err := v.Decode(&s); err != nil {
				errs = append(errs, errf("schema", "not an integer: %v", err))
			} else {
				f.Schema = s
			}
		case "environment":
			envNode = v
		case "agent":
			agentNode = v
		default:
			errs = append(errs, errf(k.Value, "unknown top-level field (supported: schema, environment, agent)"))
		}
	}

	if envNode != nil {
		if err := f.parseEnvironment(envNode); err != nil {
			errs = append(errs, err)
		}
	}
	if agentNode != nil {
		agentData, err := yaml.Marshal(agentNode)
		if err != nil {
			errs = append(errs, errf("agent", "decoding: %v", err))
		} else if err := strictDecode(agentData, &f.Agent); err != nil {
			errs = append(errs, errf("agent", "%v", err))
		}
	}

	errs = append(errs, f.validate()...)
	if len(errs) > 0 {
		return nil, errs
	}
	return f, nil
}

// parseEnvironment walks the environment mapping: the common fields are
// decoded here; exactly one driver-owned section (the key matching base) is
// preserved raw for the driver.
func (f *File) parseEnvironment(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return errf("environment", "expected a mapping")
	}
	env := &f.Environment
	var sectionNode *yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		k, v := node.Content[i], node.Content[i+1]
		switch k.Value {
		case "base":
			var s string
			if err := v.Decode(&s); err != nil {
				return errf("environment.base", "not a string: %v", err)
			}
			env.Base = s
		case "fakeintake":
			var b bool
			if err := v.Decode(&b); err != nil {
				return errf("environment.fakeintake", "not a boolean: %v", err)
			}
			env.FakeIntake = &b
		default:
			if env.Base != "" && k.Value == env.Base {
				// the driver-owned section, named after the base
				if sectionNode != nil {
					return errf("environment."+k.Value, "duplicated")
				}
				sectionNode = v
				continue
			}
			if env.Base == "" {
				return errf("environment.base", "must come before the %q section so the section can be attributed to a base", k.Value)
			}
			return errf("environment."+k.Value, "not supported for base %q (only the %q section is)", env.Base, env.Base)
		}
	}
	if sectionNode != nil {
		data, err := yaml.Marshal(sectionNode)
		if err != nil {
			return errf("environment."+env.Base, "decoding section: %v", err)
		}
		env.Section = data
	}
	return nil
}

// validate returns every validation error found in the file (generic rules
// only; driver sections are validated by their drivers, installer rules by
// the installers).
func (f *File) validate() []error {
	var errs []error

	if f.Schema == 0 {
		errs = append(errs, errf("schema", "missing (must be %d)", SchemaVersion))
	} else if f.Schema != SchemaVersion {
		errs = append(errs, errf("schema", "unsupported version %d (supported: %d)", f.Schema, SchemaVersion))
	}
	if f.Environment.Base == "" {
		errs = append(errs, errf("environment.base", "missing"))
	}

	a := &f.Agent
	if a.Install == "" {
		errs = append(errs, errf("agent.install", "missing"))
	}
	if a.Version != "" && !versionRegexp.MatchString(a.Version) {
		errs = append(errs, errf("agent.version", "%q is not a released agent version (expected e.g. \"7.69.0\")", a.Version))
	}
	if a.Image != "" {
		if !imageRefRegexp.MatchString(a.Image) {
			errs = append(errs, errf("agent.image",
				"%q is not a fully-qualified image reference with tag (expected e.g. \"gcr.io/datadoghq/agent:7.99.0-e2ectl\")", a.Image))
		} else if !semverTagRegexp.MatchString(imageTag(a.Image)) {
			// The Datadog Helm chart derives feature comparisons (semverCompare)
			// from the agent image tag, so the tag must parse as semver.
			errs = append(errs, errf("agent.image",
				"tag %q is not semver-shaped (expected e.g. \"7.99.0-e2ectl\"; the Helm chart runs version comparisons on it)", imageTag(a.Image)))
		}
	}
	if a.Config != "" {
		var cfg map[string]any
		if err := yaml.Unmarshal([]byte(a.Config), &cfg); err != nil {
			errs = append(errs, errf("agent.config", "not valid YAML: %v", err))
		}
	}
	for folder := range a.Integrations {
		if !integrationPattern.MatchString(folder) {
			errs = append(errs, errf("agent.integrations",
				"%q is not a valid conf.d folder name (expected e.g. \"custom_logs.d\")", folder))
		}
	}
	return errs
}

// StrictDecode strict-decodes raw YAML into out (unknown fields are errors).
// Drivers use it for their own config sections.
func StrictDecode(raw []byte, out any) error {
	return strictDecode(raw, out)
}

func strictDecode(raw []byte, out any) error {
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

// Errors aggregates validation errors.
type Errors struct {
	errs []error
}

func (e *Errors) Error() string {
	parts := make([]string, 0, len(e.errs))
	for _, err := range e.errs {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "\n")
}

func errf(field, format string, args ...any) error {
	return fmt.Errorf("%s: %s", field, fmt.Sprintf(format, args...))
}

// NewErrors wraps validation errors collected outside config (driver sections,
// installer rules) into the same aggregate error.
func NewErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	return &Errors{errs: errs}
}
