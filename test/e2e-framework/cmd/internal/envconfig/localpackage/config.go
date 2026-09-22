// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localpackage owns the agent.package installer section — now internal:
// the simplified agent surface derives it, users never write it. One schema
// serves both installation targets (local Docker container filesystem and
// remote SSH host); target-specific availability is enforced at install time,
// not by splitting the schema.
package localpackage

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	binaryconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/binary"
)

// Config is the unified DEB installer section. sha256 pins an exactly selected
// local DEB; derived pipeline selections omit it — the pipeline download
// provider computes and verifies the digest of the downloaded file itself,
// and every installed package still carries a verified digest in its receipt.
type Config struct {
	AllowUnsigned bool              `yaml:"allow-unsigned" description:"Explicit permission to install the selected unsigned local DEB (no repository policy changes)."`
	SHA256        string            `yaml:"sha256,omitempty" description:"Exact DEB SHA256 when a specific local package is selected; derived pipeline selections pin the downloaded file in the provider receipt." example:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`
	Config        string            `yaml:"config,omitempty" description:"Extra datadog.yaml configuration."`
	Integrations  map[string]string `yaml:"integrations,omitempty" description:"Integration folder to conf.yaml content (SSH host targets only; container targets run the fixed core checks)."`
}
type Rules struct{}

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (Rules) Validate(c Config) error {
	if c.SHA256 != "" && !digestPattern.MatchString(c.SHA256) {
		return fmt.Errorf("sha256 must be an exact lowercase SHA256")
	}
	return (binaryconfig.Rules{}).Validate(binaryconfig.Config{Config: c.Config, Integrations: c.Integrations})
}

var Schema = configschema.Must[Config](Rules{})
