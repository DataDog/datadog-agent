// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent defines the fx options for the core agent
package agent

import (
	_ "expvar"         // Blank import used because this isn't directly used in this file
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"go.uber.org/fx"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	runcmd "github.com/DataDog/datadog-agent/cmd/agent/subcommands/run"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit"
	"github.com/DataDog/datadog-agent/comp/agent/cloudfoundrycontainer"
	"github.com/DataDog/datadog-agent/comp/agent/expvarserver"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/gui"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	langDetectionCl "github.com/DataDog/datadog-agent/comp/languagedetection/client"
	langDetectionClimpl "github.com/DataDog/datadog-agent/comp/languagedetection/client/clientimpl"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	haagentmetadata "github.com/DataDog/datadog-agent/comp/metadata/haagent/def"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryotel"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	securityagentmetadata "github.com/DataDog/datadog-agent/comp/metadata/securityagent/def"
	systemprobemetadata "github.com/DataDog/datadog-agent/comp/metadata/systemprobe/def"
	"github.com/DataDog/datadog-agent/comp/ndmtmp"
	"github.com/DataDog/datadog-agent/comp/netflow"
	netflowServer "github.com/DataDog/datadog-agent/comp/netflow/server"
	"github.com/DataDog/datadog-agent/comp/networkpath"
	otelcollector "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	otelagentStatusfx "github.com/DataDog/datadog-agent/comp/otelcol/status/fx"
	"github.com/DataDog/datadog-agent/comp/process"
	processAgent "github.com/DataDog/datadog-agent/comp/process/agent"
	processagentStatusImpl "github.com/DataDog/datadog-agent/comp/process/status/statusimpl"
	rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	snmpscanfx "github.com/DataDog/datadog-agent/comp/snmpscan/fx"
	"github.com/DataDog/datadog-agent/comp/snmptraps"
	snmptrapsServer "github.com/DataDog/datadog-agent/comp/snmptraps/server"
	syntheticsTestsfx "github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/fx"
	traceagentStatusImpl "github.com/DataDog/datadog-agent/comp/trace/status/statusimpl"
	ssistatusfx "github.com/DataDog/datadog-agent/comp/updater/ssistatus/fx"
	profileStatus "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/status"
	hostSbom "github.com/DataDog/datadog-agent/pkg/sbom/collectors/host"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	clusteragentStatus "github.com/DataDog/datadog-agent/pkg/status/clusteragent"
	jmxStatus "github.com/DataDog/datadog-agent/pkg/status/jmx"
	systemprobeStatus "github.com/DataDog/datadog-agent/pkg/status/systemprobe"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	return runcmd.Commands(globalParams, run, getExtraFxOptions())
}

// GetExtraFxOptions returns the extra fx options for the agent
func getExtraFxOptions() fx.Option {
	return fx.Options(
		fx.Provide(func() flaretypes.Provider {
			return flaretypes.NewProvider(hostSbom.FlareProvider)
		}),
		fx.Supply(
			status.NewInformationProvider(jmxStatus.Provider{}),
			status.NewInformationProvider(profileStatus.Provider{}),
		),
		fx.Provide(func(config config.Component) status.InformationProvider {
			return status.NewInformationProvider(clusteragentStatus.GetProvider(config))
		}),
		fx.Provide(func(sysprobeconfig sysprobeconfig.Component) status.InformationProvider {
			return status.NewInformationProvider(systemprobeStatus.GetProvider(sysprobeconfig))
		}),
		otelagentStatusfx.Module(),
		traceagentStatusImpl.Module(),
		processagentStatusImpl.Module(),
		ndmtmp.Bundle(),
		netflow.Bundle(),
		rdnsquerierfx.Module(),
		snmptraps.Bundle(),
		snmpscanfx.Module(),
		process.Bundle(),
		networkpath.Bundle(),
		syntheticsTestsfx.Module(),
		ssistatusfx.Module(),
		langDetectionClimpl.Module(),
	)
}

// run starts the main loop.
func run(
	log log.Component,
	cfg config.Component,
	flare flare.Component,
	telemetry telemetry.Component,
	sysprobeconfig sysprobeconfig.Component,
	server dogstatsdServer.Component,
	replay replay.Component,
	forwarder defaultforwarder.Component,
	wmeta workloadmeta.Component,
	filterStore workloadfilter.Component,
	taggerComp tagger.Component,
	ac autodiscovery.Component,
	rcclient rcclient.Component,
	runner runner.Component,
	demultiplexer demultiplexer.Component,
	serializer serializer.MetricSerializer,
	logsAgent option.Option[logsAgent.Component],
	statsd statsd.Component,
	_ processAgent.Component,
	otelCollector otelcollector.Component,
	host host.Component,
	inventoryagent inventoryagent.Component,
	inventoryhost inventoryhost.Component,
	inventoryotel inventoryotel.Component,
	haagentmetadata haagentmetadata.Component,
	secrets secrets.Component,
	invChecks inventorychecks.Component,
	logReceiver option.Option[integrations.Component],
	_ netflowServer.Component,
	_ snmptrapsServer.Component,
	_ langDetectionCl.Component,
	internalAPI internalAPI.Component,
	packagesigning packagesigning.Component,
	_ systemprobemetadata.Component,
	_ securityagentmetadata.Component,
	status status.Component,
	collector collector.Component,
	_ cloudfoundrycontainer.Component,
	expvarserver expvarserver.Component,
	pid pid.Component,
	jmxlogger jmxlogger.Component,
	healthprobe healthprobe.Component,
	autoexit autoexit.Component,
	settings settings.Component,
	gui option.Option[gui.Component],
	agenttelemetryComponent agenttelemetry.Component,
	diagnose diagnose.Component,
	hostname hostnameinterface.Component,
	ipc ipc.Component,
) error {
	return runcmd.RunCore(
		log,
		cfg,
		flare,
		telemetry,
		sysprobeconfig,
		server,
		replay,
		forwarder,
		wmeta,
		filterStore,
		taggerComp,
		ac,
		rcclient,
		runner,
		demultiplexer,
		serializer,
		logsAgent,
		statsd,
		otelCollector,
		host,
		inventoryagent,
		inventoryhost,
		inventoryotel,
		haagentmetadata,
		secrets,
		invChecks,
		logReceiver,
		internalAPI,
		packagesigning,
		status,
		collector,
		expvarserver,
		pid,
		jmxlogger,
		healthprobe,
		autoexit,
		settings,
		gui,
		agenttelemetryComponent,
		diagnose,
		hostname,
		ipc,
	)
}
