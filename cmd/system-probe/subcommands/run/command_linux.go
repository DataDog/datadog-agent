// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package run

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTaggerFx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote"
	remoteWorkloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-remote"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-remote"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	connectionsforwarderfx "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	localtraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-local"
	rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	compliance "github.com/DataDog/datadog-agent/comp/system-probe/compliance/fx"
	discovery "github.com/DataDog/datadog-agent/comp/system-probe/discovery/fx"
	dynamicinstrumentation "github.com/DataDog/datadog-agent/comp/system-probe/dynamicinstrumentation/fx"
	ebpf "github.com/DataDog/datadog-agent/comp/system-probe/ebpf/fx"
	eventmonitor "github.com/DataDog/datadog-agent/comp/system-probe/eventmonitor/fx"
	gpu "github.com/DataDog/datadog-agent/comp/system-probe/gpu/fx"
	languagedetection "github.com/DataDog/datadog-agent/comp/system-probe/languagedetection/fx"
	networktracer "github.com/DataDog/datadog-agent/comp/system-probe/networktracer/fx"
	oomkill "github.com/DataDog/datadog-agent/comp/system-probe/oomkill/fx"
	ping "github.com/DataDog/datadog-agent/comp/system-probe/ping/fx"
	privilegedlogs "github.com/DataDog/datadog-agent/comp/system-probe/privilegedlogs/fx"
	process "github.com/DataDog/datadog-agent/comp/system-probe/process/fx"
	tcpqueuelength "github.com/DataDog/datadog-agent/comp/system-probe/tcpqueuelength/fx"
	traceroute "github.com/DataDog/datadog-agent/comp/system-probe/traceroute/fx"
)

func getPlatformModules() fx.Option {
	return fx.Options(
		wmcatalog.GetCatalog(),
		workloadmetafx.Module(workloadmeta.Params{
			AgentType: workloadmeta.Remote,
		}),
		connectionsforwarderfx.Module(),
		eventplatformreceiverimpl.Module(),
		eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
		rdnsquerierfx.Module(),
		npcollectorimpl.Module(),
		remoteTaggerFx.Module(tagger.NewRemoteParams()),
		remoteWorkloadfilterfx.Module(),
		remotehostnameimpl.Module(),
		logscompressionfx.Module(),
		localtraceroute.Module(),

		// system-probe modules
		networktracer.Module(),
		ebpf.Module(),
		tcpqueuelength.Module(),
		oomkill.Module(),
		compliance.Module(),
		discovery.Module(),
		dynamicinstrumentation.Module(),
		gpu.Module(),
		languagedetection.Module(),
		ping.Module(),
		privilegedlogs.Module(),
		process.Module(),
		traceroute.Module(),
		eventmonitor.Module(),
	)
}
