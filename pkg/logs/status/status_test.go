// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package status

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/stretchr/testify/assert"
)

func TestSourceAreGroupedByIntegrations(t *testing.T) {
	sources := []*config.LogSource{
		config.NewLogSource("foo", &config.LogsConfig{}),
		config.NewLogSource("bar", &config.LogsConfig{}),
		config.NewLogSource("foo", &config.LogsConfig{}),
	}
	Initialize(sources, nil)
	status := Get()
	assert.Equal(t, true, status.IsRunning)
	assert.Equal(t, 2, len(status.Integrations))
	assert.Equal(t, 0, len(status.DeprecatedAttributes))

	for _, integration := range status.Integrations {
		switch integration.Name {
		case "foo":
			assert.Equal(t, 2, len(integration.Sources))
		case "bar":
			assert.Equal(t, 1, len(integration.Sources))
		default:
			assert.Fail(t, fmt.Sprintf("Expected foo or bar, got %s", integration.Name))
		}
	}
}

func TestSourceAreGroupedByIntegrations(t *testing.T) {
	deprecatedAttributes := []config.DeprecatedAttribute{
		{
			Name:        "foo",
			Replacement: "boo",
		},
		{
			Name:        "bar",
			Replacement: "baz",
		},
	}
	Initialize(nil, deprecatedAttributes)
	status := Get()
	assert.Equal(t, true, status.IsRunning)
	assert.Equal(t, 0, len(status.Integrations))
	assert.Equal(t, 2, len(status.DeprecatedAttributes))

	for _, attribute := range status.DeprecatedAttributes {
		switch attribute.Name {
		case "foo":
			assert.Equal(t, "boo", attribute.Replacement)
		case "bar":
			assert.Equal(t, "baz", attribute.Replacement)
		default:
			assert.Fail(t, fmt.Sprintf("Expected foo or bar, got %s", attribute.Name))
		}
	}
}
