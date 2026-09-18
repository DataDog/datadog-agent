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

type Config struct {
	URL string `yaml:"url" config:"required" example:"http://sink.example.test:8080" description:"Externally managed HTTP(S) sink reachable from the Agent network."`
}
type Rules struct{}

func (Rules) Validate(c Config) error {
	_, err := receivers.EndpointURL(c.URL)
	return err
}

var Schema = configschema.Must[Config](Rules{})
