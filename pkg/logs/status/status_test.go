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
	sources := config.NewLogSources()
	sources.AddSource(config.NewLogSource("foo", &config.LogsConfig{}))
	sources.AddSource(config.NewLogSource("bar", &config.LogsConfig{}))
	sources.AddSource(config.NewLogSource("foo", &config.LogsConfig{}))
	Initialize(sources)
	status := Get()
	assert.Equal(t, true, status.IsRunning)
	assert.Equal(t, 2, len(status.Integrations))

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
