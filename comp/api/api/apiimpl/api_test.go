// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"context"
	"testing"

	// component dependencies
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/pidmapimpl"
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-mock"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"

	// package dependencies

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	// third-party dependencies

	"go.uber.org/fx"
)

func getComponentDependencies(t *testing.T) api.Component {
	// TODO: this fxutil.Test[T] can take a component and return the component
	return fxutil.Test[api.Component](
		t,
		Module(),
		pidmapimpl.Module(),
		hostnameimpl.MockModule(),
		dogstatsdServer.MockModule(),
		replaymock.MockModule(),
		secretsimpl.MockModule(),
		nooptelemetry.Module(),
		demultiplexerimpl.MockModule(),
		fx.Supply(optional.NewNoneOption[rcservice.Component]()),
		fx.Supply(optional.NewNoneOption[rcservicemrf.Component]()),
		fetchonlyimpl.MockModule(),
		fx.Supply(context.Background()),
		taggerimpl.MockModule(),
		fx.Provide(func(mock tagger.Mock) tagger.Component { return mock }),
		fx.Supply(autodiscoveryimpl.MockParams{Scheduler: nil}),
		autodiscoveryimpl.MockModule(),
		fx.Provide(func(mock autodiscovery.Mock) autodiscovery.Component { return mock }),
		fx.Supply(optional.NewNoneOption[logsAgent.Component]()),
		fx.Supply(optional.NewNoneOption[collector.Component]()),
		// Ensure we pass a nil endpoint to test that we always filter out nil endpoints
		fx.Provide(func() api.AgentEndpointProvider {
			return api.AgentEndpointProvider{
				Provider: nil,
			}
		}),
	)
}

func TestStartServer(t *testing.T) {
	getComponentDependencies(t)
}
