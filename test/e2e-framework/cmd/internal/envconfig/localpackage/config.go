// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localpackage owns agent.package installer settings, not its source.
package localpackage

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	binaryconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/binary"
)

type Config struct {
	AllowUnsigned bool              `yaml:"allow-unsigned" description:"Explicit permission to install the selected local unsigned DEB (no repository policy changes)."`
	Config        string            `yaml:"config,omitempty" description:"Extra datadog.yaml configuration."`
	Integrations  map[string]string `yaml:"integrations,omitempty" description:"Integration folder to conf.yaml content."`
}
type Rules struct{}

func (Rules) Validate(c Config) error {
	return (binaryconfig.Rules{}).Validate(binaryconfig.Config{Config: c.Config, Integrations: c.Integrations})
}

var Schema = configschema.Must[Config](Rules{})
