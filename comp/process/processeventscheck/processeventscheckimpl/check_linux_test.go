// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package processeventscheckimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/process/processeventscheck"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestProcessEventsCheckIsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		configs map[string]interface{}
		enabled bool
	}{
		{
			name:    "config not set",
			configs: map[string]interface{}{},
			enabled: false,
		},
		{
			name: "config is disabled",
			configs: map[string]interface{}{
				"process_config.event_collection.enabled": false,
			},
			enabled: false,
		},
		{
			name: "config is enabled",
			configs: map[string]interface{}{
				"process_config.event_collection.enabled": true,
			},
			enabled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := fxutil.Test[processeventscheck.Component](t, fx.Options(
				core.MockBundle(),
				fx.Replace(config.MockParams{Overrides: tc.configs}),
				fx.Provide(func() statsd.ClientInterface {
					return &statsd.NoOpClient{}
				}),
				Module(),
			))

			assert.Equal(t, tc.enabled, c.Object().IsEnabled())
		})
	}
}
