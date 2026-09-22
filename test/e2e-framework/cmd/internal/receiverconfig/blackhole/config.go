// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package blackhole owns the blackhole receiver selection schema.
package blackhole

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
)

// URL is an optional escape hatch: empty selects the environment-managed sink
// (the local base starts it during install/apply), non-empty is an operator-
// owned sink that must outlive the Agent installation.
type Config struct {
	URL string `yaml:"url" example:"http://sink.example.test:8080" description:"Externally managed HTTP(S) sink reachable from the Agent network. Empty selects the environment-managed sink (local base)."`
}
type Rules struct{}

func (Rules) Validate(c Config) error {
	if c.URL == "" {
		return nil
	}
	_, err := receivers.EndpointURL(c.URL)
	return err
}

var Schema = configschema.Must[Config](Rules{})
