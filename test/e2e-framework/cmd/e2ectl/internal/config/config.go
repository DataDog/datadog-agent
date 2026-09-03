// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package config defines and validates the e2ectl environment configuration
// (schema v1). Validation is deliberately strict: unknown fields, invalid
// enums and invalid cross-field combinations all fail with field-anchored
// errors, before any environment is touched.
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

// Bases supported by environment.base.
const (
	BaseKind    = "kind"
	BaseEC2Host = "ec2-host"
)

// Install methods supported by agent.install.
const (
	InstallHelm   = "helm"
	InstallScript = "script"
)

// SupportedOS is the list of host OS descriptors accepted for ec2-host.
// The design document calls for deriving this from components/os (e2eos)
// so the list can never drift; that package currently pulls in Pulumi
// transitively, so the core CLI keeps this table and the worker maps it to
// e2eos descriptors (see qa-e2ectl-implementation-notes.md).
var SupportedOS = []string{"ubuntu-22.04", "ubuntu-24.04"}

// SupportedArch is the list of architectures accepted for ec2-host.
var SupportedArch = []string{"amd64", "arm64"}

// File is an e2ectl environment configuration file.
type File struct {
	Schema      int         `yaml:"schema"`
	Environment Environment `yaml:"environment"`
	Agent       Agent       `yaml:"agent"`

	// Path is the file this config was loaded from; set by Load, not part of
	// the YAML schema.
	Path string `yaml:"-"`
}

// Environment describes the environment to create.
type Environment struct {
	Base       string      `yaml:"base"`
	Kubernetes *Kubernetes `yaml:"kubernetes,omitempty"`
	VM         *VM         `yaml:"vm,omitempty"`
	// FakeIntake defaults to true. Pointer so "explicitly false" is representable.
	FakeIntake *bool `yaml:"fakeintake,omitempty"`
}

// Kubernetes carries kind-specific settings.
type Kubernetes struct {
	// Version is a full kindest/node tag version, e.g. "1.31.0".
	Version string `yaml:"version,omitempty"`
	// Nodes is the number of worker nodes in addition to the control plane.
	Nodes int `yaml:"nodes,omitempty"`
}

// VM carries ec2-host specific settings.
type VM struct {
	OS           string `yaml:"os,omitempty"`
	Arch         string `yaml:"arch,omitempty"`
	InstanceType string `yaml:"instance-type,omitempty"`
}

// Agent describes how the Datadog agent is installed on the environment.
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
	kindVersionRegexp  = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	agentVersionRegexp = kindVersionRegexp
	integrationPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+\.d$`)
	imageRefRegexp     = regexp.MustCompile(`^[a-zA-Z0-9.-]+(:[0-9]+)?/[a-zA-Z0-9/._-]+:[a-zA-Z0-9._-]+$`)
)

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
	var errs []error

	node := yaml.Node{}
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, []error{fmt.Errorf("invalid YAML: %w", err)}
	}

	f := &File{}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(f); err != nil {
		return nil, []error{fmt.Errorf("decoding config: %w", err)}
	}

	errs = append(errs, f.validate()...)
	if len(errs) > 0 {
		return nil, errs
	}
	return f, nil
}

// validate returns every validation error found in the file.
func (f *File) validate() []error {
	var errs []error

	if f.Schema == 0 {
		errs = append(errs, errf("schema", "missing (must be %d)", SchemaVersion))
	} else if f.Schema != SchemaVersion {
		errs = append(errs, errf("schema", "unsupported version %d (supported: %d)", f.Schema, SchemaVersion))
	}

	errs = append(errs, f.validateEnvironment()...)
	errs = append(errs, f.validateAgent()...)
	return errs
}

func (f *File) validateEnvironment() []error {
	var errs []error
	env := &f.Environment

	switch env.Base {
	case "":
		errs = append(errs, errf("environment.base", "missing (supported: %s, %s)", BaseKind, BaseEC2Host))
	case BaseKind:
		if env.VM != nil {
			errs = append(errs, errf("environment.vm", "not supported for base %q", BaseKind))
		}
		if env.Kubernetes != nil {
			if env.Kubernetes.Version != "" && !kindVersionRegexp.MatchString(env.Kubernetes.Version) {
				errs = append(errs, errf("environment.kubernetes.version",
					"%q is not a full version (expected e.g. \"1.31.0\", it maps to kindest/node:v1.31.0)", env.Kubernetes.Version))
			}
			if env.Kubernetes.Nodes < 0 {
				errs = append(errs, errf("environment.kubernetes.nodes", "must be >= 0"))
			}
		}
	case BaseEC2Host:
		if env.Kubernetes != nil {
			errs = append(errs, errf("environment.kubernetes", "not supported for base %q", BaseEC2Host))
		}
		if env.VM != nil {
			if !contains(SupportedOS, env.VM.OS) {
				errs = append(errs, errf("environment.vm.os", "%q is not supported (supported: %s)",
					env.VM.OS, strings.Join(SupportedOS, ", ")))
			}
			if !contains(SupportedArch, env.VM.Arch) {
				errs = append(errs, errf("environment.vm.arch", "%q is not supported (supported: %s)",
					env.VM.Arch, strings.Join(SupportedArch, ", ")))
			}
		} else {
			errs = append(errs, errf("environment.vm", "required for base %q", BaseEC2Host))
		}
	default:
		errs = append(errs, errf("environment.base", "%q is not supported (supported: %s, %s)",
			env.Base, BaseKind, BaseEC2Host))
	}
	return errs
}

func (f *File) validateAgent() []error {
	var errs []error
	a := &f.Agent

	switch a.Install {
	case "":
		errs = append(errs, errf("agent.install", "missing (supported: %s, %s)", InstallHelm, InstallScript))
	case InstallHelm:
		if f.Environment.Base != BaseKind {
			errs = append(errs, errf("agent.install", "%q is not supported for base %q (supported: %s)",
				InstallHelm, f.Environment.Base, InstallScript))
		}
		if a.Version == "" && a.Image == "" {
			errs = append(errs, errf("agent", "either version or image is required when install is %q", InstallHelm))
		}
		if a.Version != "" && !kindVersionRegexp.MatchString(a.Version) {
			errs = append(errs, errf("agent.version", "%q is not a released agent version (expected e.g. \"7.69.0\")", a.Version))
		}
		if a.Image != "" && !imageRefRegexp.MatchString(a.Image) {
			errs = append(errs, errf("agent.image",
				"%q is not a fully-qualified image reference with tag (expected e.g. \"gcr.io/datadoghq/agent:my-dev\")", a.Image))
		}
	case InstallScript:
		if f.Environment.Base != BaseEC2Host {
			errs = append(errs, errf("agent.install", "%q is not supported for base %q (supported: %s)",
				InstallScript, f.Environment.Base, InstallHelm))
		}
		if a.Version == "" {
			errs = append(errs, errf("agent.version", "required when install is %q", InstallScript))
		}
		if a.Version != "" && !agentVersionRegexp.MatchString(a.Version) {
			errs = append(errs, errf("agent.version", "%q is not a released agent version (expected e.g. \"7.69.0\")", a.Version))
		}
		if a.Image != "" {
			errs = append(errs, errf("agent.image", "not supported when install is %q", InstallScript))
		}
	default:
		errs = append(errs, errf("agent.install", "%q is not supported (supported: %s, %s)",
			a.Install, InstallHelm, InstallScript))
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

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
