// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fixtures owns the common fixture options, not provider-specific fields.
package fixtures

import "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"

type Config struct {
	// No json omitempty: explicit false must survive the executor boundary.
	FakeIntake bool `yaml:"fakeintake" json:"fakeintake" default:"true" description:"Deploy fakeintake with the environment (Pulumi for EC2, local Docker for kind)."`
}

var Schema = configschema.Must[Config]()

// UnmarshalJSON is the normalized executor boundary, not user-YAML parsing.
// Validate with the same schema before Go can erase omission/null presence.
func (c *Config) UnmarshalJSON(data []byte) error {
	value, err := Schema.DecodeResolved(data, "fixtures")
	if err != nil {
		return err
	}
	*c = value
	return nil
}
