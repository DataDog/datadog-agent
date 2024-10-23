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
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
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

	mockTagger := taggerimpl.SetupFakeTagger(t)

	deps := fxutil.Test[dependencies](t, fx.Options(
		core.MockBundle(),
		compressionimpl.MockModule(),
		defaultforwarder.MockModule(),
		orchestratorimpl.MockModule(),
		eventplatformimpl.MockModule(),
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
