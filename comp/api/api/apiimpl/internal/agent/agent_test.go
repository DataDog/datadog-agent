// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	// component dependencies
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"

	// package dependencies
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	// third-party dependencies
	"github.com/gorilla/mux"
	"go.uber.org/fx"
)

type handlerdeps struct {
	fx.In

	Wmeta          workloadmeta.Component
	LogsAgent      optional.Option[logsAgent.Component]
	HostMetadata   host.Component
	SecretResolver secrets.Component
	Demux          demultiplexer.Component
	Collector      optional.Option[collector.Component]
	Ac             autodiscovery.Mock
	Tagger         taggermock.Mock
}

func getComponentDeps(t *testing.T) handlerdeps {
	return fxutil.Test[handlerdeps](
		t,
		fx.Supply(context.Background()),
		hostnameinterface.MockModule(),
		fx.Provide(func() optional.Option[logsAgent.Component] {
			return optional.NewNoneOption[logsAgent.Component]()
		}),
		hostimpl.MockModule(),
		demultiplexerimpl.MockModule(),
		secretsimpl.MockModule(),
		fx.Provide(func() optional.Option[collector.Component] {
			return optional.NewNoneOption[collector.Component]()
		}),
		taggermock.Module(),
		fx.Options(
			fx.Supply(autodiscoveryimpl.MockParams{Scheduler: nil}),
			autodiscoveryimpl.MockModule(),
		),
		config.MockModule(),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		workloadmetafx.Module(workloadmeta.NewParams()),
		telemetryimpl.MockModule(),
	)
}

func setupRoutes(t *testing.T) *mux.Router {
	deps := getComponentDeps(t)
	sender := aggregator.NewNoOpSenderManager()

	apiProviders := []api.EndpointProvider{
		api.NewAgentEndpointProvider(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("OK"))
		}, "/dynamic_route", "GET").Provider,
	}

	router := mux.NewRouter()
	SetupHandlers(
		router,
		deps.Wmeta,
		deps.LogsAgent,
		sender,
		deps.SecretResolver,
		deps.Collector,
		deps.Ac,
		apiProviders,
		deps.Tagger,
	)

	return router
}

func TestSetupHandlers(t *testing.T) {
	testcases := []struct {
		route    string
		method   string
		wantCode int
	}{
		{
			route:    "/dynamic_route",
			method:   "GET",
			wantCode: 200,
		},
	}
	router := setupRoutes(t)
	ts := httptest.NewServer(router)
	defer ts.Close()

	for _, tc := range testcases {
		req, err := http.NewRequest(tc.method, ts.URL+tc.route, nil)
		require.NoError(t, err)

		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		assert.Equal(t, tc.wantCode, resp.StatusCode, "%s %s failed with a %d, want %d", tc.method, tc.route, resp.StatusCode, tc.wantCode)
	}
}
