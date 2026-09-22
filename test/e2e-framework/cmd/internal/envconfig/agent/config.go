// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent describes the user-facing agent section (agent:) — the
// simplified installation surface. Users pick at most ONE source (source,
// pipeline or version); the install mechanism and the artifact build provider
// are DERIVED from (source, environment base) by Derive. The installer-owned
// sections (agent.binary, agent.helm, ...) still exist, but they are internal:
// the derivation produces them, users cannot write them.
package agent

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	binaryconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/binary"
	"go.yaml.in/yaml/v3"
)

// Config is the whole user-facing agent input: one optional source selector
// plus fields that work on every mechanism.
type Config struct {
	Source       bool              `yaml:"source,omitempty" description:"Build the Agent from this checkout."`
	Pipeline     int               `yaml:"pipeline,omitempty" example:"138372337" description:"Install the CI pipeline's DEB artifacts (local and ec2-host bases)."`
	Version      string            `yaml:"version,omitempty" example:"7.83.0" description:"Install a released agent version (kind and ec2-host bases)."`
	Config       string            `yaml:"config,omitempty" description:"Extra datadog.yaml configuration."`
	Integrations map[string]string `yaml:"integrations,omitempty" description:"conf.d folder name -> its conf.yaml contents."`
	Values       string            `yaml:"values,omitempty" description:"Extra Helm chart values, deep-merged over the installer's defaults (kind base only)."`
}

var releasedVersionRegexp = regexp.MustCompile(`^[0-9]+[.][0-9]+[.][0-9]+$`)

// Rules checks what annotations cannot: the sources are mutually exclusive,
// the version is a released version, and the common config/integrations/values
// fields are well-formed — the same shared rules the installer sections apply.
type Rules struct{}

func (Rules) Validate(c Config) error {
	set := 0
	if c.Source {
		set++
	}
	if c.Pipeline != 0 {
		set++
	}
	if c.Version != "" {
		set++
	}
	if set > 1 {
		return fmt.Errorf("source, pipeline and version are mutually exclusive: pick exactly one (or none for the environment default)")
	}
	if c.Version != "" && !releasedVersionRegexp.MatchString(c.Version) {
		return fmt.Errorf("version: %q is not a released agent version (expected e.g. \"7.83.0\")", c.Version)
	}
	if c.Pipeline < 0 {
		return fmt.Errorf("pipeline must be a positive pipeline id")
	}
	if c.Values != "" {
		var values map[string]any
		if err := yaml.Unmarshal([]byte(c.Values), &values); err != nil {
			return fmt.Errorf("values: not valid YAML chart values")
		}
	}
	// The shared host-side rules: integration folder names and the embedded
	// agent config being valid YAML.
	return (binaryconfig.Rules{}).Validate(binaryconfig.Config{Config: c.Config, Integrations: c.Integrations})
}

// Schema is shared by the config envelope parser and the derivation.
var Schema = configschema.Must[Config](Rules{})
