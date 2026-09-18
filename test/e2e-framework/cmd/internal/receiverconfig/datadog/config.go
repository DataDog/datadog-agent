// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package datadog owns the datadog receiver selection schema.
package datadog

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
)

type Config struct {
	Site      string `yaml:"site" config:"required" example:"datadoghq.eu" description:"Explicit Datadog site; real telemetry will leave the environment."`
	APIKeyRef string `yaml:"api-key-ref" config:"required" enum:"runner/api_key" example:"runner/api_key" description:"Bounded runner ingestion credential reference."`
}
type Rules struct{}

func (Rules) Validate(c Config) error {
	_, err := receivers.Datadog(c.Site, c.APIKeyRef)
	return err
}

var Schema = configschema.Must[Config](Rules{})
