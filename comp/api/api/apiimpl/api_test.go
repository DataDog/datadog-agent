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
	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/flareimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/inventorychecksimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/inventoryhostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning/packagesigningimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcserviceha"

	// package dependencies
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	// third-party dependencies
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func getProvides(t *testing.T) api.Component {
	return newAPIServer(
		fxutil.Test[dependencies](
			t,
			hostnameinterface.MockModule(),
			logimpl.MockModule(),
			config.MockModule(),
			flareimpl.MockModule(),
			dogstatsdServer.MockModule(),
			replay.MockModule(),
			dogstatsddebug.MockModule(),
			hostimpl.MockModule(),
			inventoryagentimpl.MockModule(),
			demultiplexerimpl.MockModule(),
			inventoryhostimpl.MockModule(),
			secretsimpl.MockModule(),
			fx.Provide(func(secretMock secrets.Mock) secrets.Component {
				component := secretMock.(secrets.Component)
				return component
			}),
			inventorychecksimpl.MockModule(),
			packagesigningimpl.MockModule(),
			statusimpl.MockModule(),
			eventplatformreceiverimpl.MockModule(),
			fx.Provide(func() optional.Option[rcservice.Component] {
				return optional.NewNoneOption[rcservice.Component]()
			}),
			fx.Provide(func() optional.Option[rcserviceha.Component] {
				return optional.NewNoneOption[rcserviceha.Component]()
			}),
			fetchonlyimpl.MockModule(),
		),
	)
}

func getTestAPIServer(t *testing.T) *apiServer {
	p := getProvides(t)
	return p.(*apiServer)
}

func TestStartServer(t *testing.T) {

	store := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(workloadmeta.NewParams()),
		fx.Supply(context.Background()),
		workloadmeta.MockModule(),
	))
	tags := fxutil.Test[tagger.Mock](t, tagger.MockModule())
	ac := autodiscoveryimpl.CreateMockAutoConfig(t, nil)
	sender := aggregator.NewNoOpSenderManager()
	log := fxutil.Test[optional.Option[logsAgent.Component]](t,
		fx.Provide(func() optional.Option[logsAgent.Component] {
			return optional.NewNoneOption[logsAgent.Component]()
		}),
	)
	col := fxutil.Test[optional.Option[collector.Component]](t,
		fx.Provide(func() optional.Option[collector.Component] {
			return optional.NewNoneOption[collector.Component]()
		}),
	)

	srv := getTestAPIServer(t)
	err := srv.StartServer(
		store,
		tags,
		ac,
		log,
		sender,
		col,
	)
	defer srv.StopServer()

	assert.NoError(t, err, "could not start api component servers: %v", err)
}
