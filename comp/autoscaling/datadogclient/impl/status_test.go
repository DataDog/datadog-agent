// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package datadogclientimpl

import (
	"bytes"
	"strings"
	"testing"

	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestStatusProvider(t *testing.T) {
	dc := fxutil.Test[datadogclient.Component](t,
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: map[string]interface{}{
			"api_key":                           "apikey123",
			"app_key":                           "appkey456",
			"external_metrics_provider.enabled": true,
			metricsRedundantEndpointConfig: []endpoint{
				{
					"api.datadoghq.eu",
					"https://api.datadoghq.eu",
					"12345",
					"67890",
				},
			},
		}}),
		fxutil.ProvideComponentConstructor(
			NewComponent,
		),
	)

	provider := statusProvider{
		dc: dc,
	}

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			provider.JSON(false, stats)

			assert.NotEmpty(t, stats)
		}},
		{"NAME", func(t *testing.T) {
			name := provider.Name()
			assert.Equal(t, name, "External Metrics Endpoints")
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.Text(false, b)

			assert.NoError(t, err)
			expectedTextOutput := `
  - URL: https://api.datadoghq.com  [Unknown]
    Last failure: Never
    Last Success: Never
  - URL: https://api.datadoghq.eu  [Unknown]
    Last failure: Never
    Last Success: Never
`
			// We replace windows line break by linux so the tests pass on every OS
			expected := strings.Replace(string(expectedTextOutput), "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)
			assert.Equal(t, expected, output)
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.HTML(false, b)

			assert.NoError(t, err)

			assert.Empty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}
