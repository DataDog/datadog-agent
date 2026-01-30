// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package run

import (
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	remoteWorkloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-remote"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-remote"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	connectionsforwarderfx "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	compliance "github.com/DataDog/datadog-agent/comp/system-probe/compliance/fx"
	discovery "github.com/DataDog/datadog-agent/comp/system-probe/discovery/fx"
	ebpf "github.com/DataDog/datadog-agent/comp/system-probe/ebpf/fx"
	networktracer "github.com/DataDog/datadog-agent/comp/system-probe/networktracer/fx"
	oomkill "github.com/DataDog/datadog-agent/comp/system-probe/oomkill/fx"
	tcpqueuelength "github.com/DataDog/datadog-agent/comp/system-probe/tcpqueuelength/fx"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

func getPlatformModules() fx.Option {
	return fx.Options(
		statsd.Module(),
		// TODO is this bad to do multiple times?
		fx.Provide(func(config config.Component, statsd statsd.Component) (ddgostatsd.ClientInterface, error) {
			return statsd.CreateForHostPort(configutils.GetBindHost(config), config.GetInt("dogstatsd_port"))
		}),
		wmcatalog.GetCatalog(),
		workloadmetafx.Module(workloadmeta.Params{
			AgentType: workloadmeta.Remote,
		}),

		// TODO should this be a bundle?
		connectionsforwarderfx.Module(),
		eventplatformreceiverimpl.Module(),
		eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
		rdnsquerierfx.Module(),
		npcollectorimpl.Module(),
		networktracer.Module(),

		ebpf.Module(),
		tcpqueuelength.Module(),
		oomkill.Module(),

		remoteWorkloadfilterfx.Module(),
		remotehostnameimpl.Module(),
		logscompressionfx.Module(),
		compliance.Module(),

		discovery.Module(),
	)
}
