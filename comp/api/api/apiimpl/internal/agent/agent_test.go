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
	demultiplexerendpointmock "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexerendpoint/fx-mock"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/flare/flareimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/inventorychecksimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/inventoryhostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning/packagesigningimpl"

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

	Server                dogstatsdServer.Component
	ServerDebug           dogstatsddebug.Component
	Wmeta                 workloadmeta.Component
	LogsAgent             optional.Option[logsAgent.Component]
	HostMetadata          host.Component
	InvAgent              inventoryagent.Component
	SecretResolver        secrets.Component
	Demux                 demultiplexer.Component
	InvHost               inventoryhost.Component
	InvChecks             inventorychecks.Component
	PkgSigning            packagesigning.Component
	Collector             optional.Option[collector.Component]
	EventPlatformReceiver eventplatformreceiver.Component
	Ac                    autodiscovery.Mock
	Tagger                tagger.Mock
	EndpointProviders     []api.EndpointProvider `group:"agent_endpoint"`
}

func getComponentDeps(t *testing.T) handlerdeps {
	return fxutil.Test[handlerdeps](
		t,
		fx.Supply(context.Background()),
		hostnameinterface.MockModule(),
		flareimpl.MockModule(),
		dogstatsdServer.MockModule(),
		serverdebugimpl.MockModule(),
		fx.Provide(func() optional.Option[logsAgent.Component] {
			return optional.NewNoneOption[logsAgent.Component]()
		}),
		hostimpl.MockModule(),
		inventoryagentimpl.MockModule(),
		demultiplexerimpl.MockModule(),
		demultiplexerendpointmock.MockModule(),
		inventoryhostimpl.MockModule(),
		secretsimpl.MockModule(),
		inventorychecksimpl.MockModule(),
		packagesigningimpl.MockModule(),
		fx.Provide(func() optional.Option[collector.Component] {
			return optional.NewNoneOption[collector.Component]()
		}),
		eventplatformreceiverimpl.MockModule(),
		taggerimpl.MockModule(),
		fx.Options(
			fx.Supply(autodiscoveryimpl.MockParams{Scheduler: nil}),
			autodiscoveryimpl.MockModule(),
		),
		settingsimpl.MockModule(),
	)
}

func setupRoutes(t *testing.T) *mux.Router {
	deps := getComponentDeps(t)
	sender := aggregator.NewNoOpSenderManager()

	router := mux.NewRouter()
	SetupHandlers(
		router,
		deps.Wmeta,
		deps.LogsAgent,
		sender,
		deps.SecretResolver,
		deps.Collector,
		deps.Ac,
		deps.EndpointProviders,
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
			route:    "/flare",
			method:   "POST",
			wantCode: 200,
		},
		{
			route:    "/stream-event-platform",
			method:   "POST",
			wantCode: 200,
		},
		{
			route:    "/dogstatsd-contexts-dump",
			method:   "POST",
			wantCode: 200,
		},
		{
			route:    "/dogstatsd-stats",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/config",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/config/list-runtime",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/config/log_level",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/config/log_level",
			method:   "POST",
			wantCode: 200,
		},
		{
			route:    "/secrets",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/secret/refresh",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/config-check",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/tagger-list",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/workload-list",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/metadata/v5",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/metadata/gohai",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/metadata/inventory-agent",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/metadata/inventory-host",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/metadata/inventory-checks",
			method:   "GET",
			wantCode: 200,
		},
		{
			route:    "/metadata/package-signing",
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
