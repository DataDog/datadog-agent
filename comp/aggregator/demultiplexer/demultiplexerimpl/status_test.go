// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package demultiplexerimpl defines the aggregator demultiplexer
package demultiplexerimpl

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	filterlistfx "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStatusOutPut(t *testing.T) {
	require := require.New(t)

	tests := []struct {
		name       string
		assertFunc func(provider status.Provider)
	}{
		{"JSON", func(provider status.Provider) {
			stats := make(map[string]interface{})
			provider.JSON(false, stats)

			require.NotEmpty(stats)
		}},
		{"Text", func(provider status.Provider) {
			b := new(bytes.Buffer)
			err := provider.Text(false, b)

			require.NoError(err)

			require.NotEmpty(b.String())
		}},
		{"HTML", func(provider status.Provider) {
			b := new(bytes.Buffer)
			err := provider.HTML(false, b)

			require.NoError(err)

			require.NotEmpty(b.String())
		}},
	}

	mockTagger := taggerfxmock.SetupFakeTagger(t)

	deps := fxutil.Test[dependencies](t, fx.Options(
		core.MockBundle(),
		hostnameimpl.MockModule(),
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		defaultforwarder.MockModule(),
		haagentmock.Module(),
		orchestratorimpl.MockModule(),
		eventplatformimpl.MockModule(),
		logscompression.MockModule(),
		metricscompression.MockModule(),
		filterlistfx.MockModule(),
		fx.Provide(func() tagger.Component {
			return mockTagger
		}),
		fx.Supply(
			Params{
				continueOnMissingHostname: true,
			},
		),
	))
	provides, err := newDemultiplexer(deps)
	require.NoError(err)

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			test.assertFunc(provides.StatusProvider.Provider)
		})
	}
}
