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

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	filterlistfx "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/mock"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformmock "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/mock"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/def"
	orchestratormock "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/mock"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// testDependencies mirrors Dependencies but uses fx.In for use with fxutil.Test.
type testDependencies struct {
	fx.In
	Lc                     compdef.Lifecycle
	Config                 config.Component
	Log                    log.Component
	SharedForwarder        defaultforwarder.Component
	OrchestratorForwarder  orchestratorforwarder.Component
	EventPlatformForwarder eventplatform.Component
	HaAgent                haagent.Component
	Compressor             compression.Component
	Tagger                 tagger.Component
	Hostname               hostnameinterface.Component
	FilterList             filterlist.Component
	Observer               observer.Component `optional:"true"`

	Params Params
}

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

	testDeps := fxutil.Test[testDependencies](t, fx.Options(
		core.MockBundle(),
		hostnameimpl.MockModule(),
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		defaultforwardermock.MockModule(),
		haagentmock.Module(),
		orchestratormock.MockModule(),
		eventplatformmock.MockModule(),
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
	deps := Dependencies{
		Lc:                     testDeps.Lc,
		Config:                 testDeps.Config,
		Log:                    testDeps.Log,
		SharedForwarder:        testDeps.SharedForwarder,
		OrchestratorForwarder:  testDeps.OrchestratorForwarder,
		EventPlatformForwarder: testDeps.EventPlatformForwarder,
		HaAgent:                testDeps.HaAgent,
		Compressor:             testDeps.Compressor,
		Tagger:                 testDeps.Tagger,
		Hostname:               testDeps.Hostname,
		FilterList:             testDeps.FilterList,
		Observer:               testDeps.Observer,
		Params:                 testDeps.Params,
	}
	provides, err := NewComponent(deps)
	require.NoError(err)

	for _, test := range tests {
		t.Run(test.name, func(_ *testing.T) {
			test.assertFunc(provides.StatusProvider.Provider)
		})
	}
}
