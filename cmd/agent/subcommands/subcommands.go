// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/agent/command"
	cmdcheck "github.com/DataDog/datadog-agent/cmd/agent/subcommands/check"
	cmdclosedsourceconsent "github.com/DataDog/datadog-agent/cmd/agent/subcommands/closedsourceconsent"
	cmdconfig "github.com/DataDog/datadog-agent/cmd/agent/subcommands/config"
	cmdconfigcheck "github.com/DataDog/datadog-agent/cmd/agent/subcommands/configcheck"
	cmdcontrolsvc "github.com/DataDog/datadog-agent/cmd/agent/subcommands/controlsvc"
	cmddiagnose "github.com/DataDog/datadog-agent/cmd/agent/subcommands/diagnose"
	cmddogstatsdcapture "github.com/DataDog/datadog-agent/cmd/agent/subcommands/dogstatsdcapture"
	cmddogstatsdreplay "github.com/DataDog/datadog-agent/cmd/agent/subcommands/dogstatsdreplay"
	cmddogstatsdstats "github.com/DataDog/datadog-agent/cmd/agent/subcommands/dogstatsdstats"
	cmdflare "github.com/DataDog/datadog-agent/cmd/agent/subcommands/flare"
	cmdhealth "github.com/DataDog/datadog-agent/cmd/agent/subcommands/health"
	cmdhostname "github.com/DataDog/datadog-agent/cmd/agent/subcommands/hostname"
	cmdimport "github.com/DataDog/datadog-agent/cmd/agent/subcommands/import"
	cmdintegrations "github.com/DataDog/datadog-agent/cmd/agent/subcommands/integrations"
	cmdjmx "github.com/DataDog/datadog-agent/cmd/agent/subcommands/jmx"
	cmdlaunchgui "github.com/DataDog/datadog-agent/cmd/agent/subcommands/launchgui"
	cmdremoteconfig "github.com/DataDog/datadog-agent/cmd/agent/subcommands/remoteconfig"
	cmdrun "github.com/DataDog/datadog-agent/cmd/agent/subcommands/run"
	cmdsecret "github.com/DataDog/datadog-agent/cmd/agent/subcommands/secret"
	cmdsecrethelper "github.com/DataDog/datadog-agent/cmd/agent/subcommands/secrethelper"
	cmdsnmp "github.com/DataDog/datadog-agent/cmd/agent/subcommands/snmp"
	cmdstatus "github.com/DataDog/datadog-agent/cmd/agent/subcommands/status"
	cmdstop "github.com/DataDog/datadog-agent/cmd/agent/subcommands/stop"
	cmdstreamlogs "github.com/DataDog/datadog-agent/cmd/agent/subcommands/streamlogs"
	cmdtaggerlist "github.com/DataDog/datadog-agent/cmd/agent/subcommands/taggerlist"
	cmdversion "github.com/DataDog/datadog-agent/cmd/agent/subcommands/version"
	cmdworkloadlist "github.com/DataDog/datadog-agent/cmd/agent/subcommands/workloadlist"
)

// AgentSubcommands returns SubcommandFactories for the subcommands supported
// with the current build flags.
func AgentSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		cmdcheck.Commands,
		cmdconfigcheck.Commands,
		cmdconfig.Commands,
		cmddiagnose.Commands,
		cmddogstatsdcapture.Commands,
		cmddogstatsdreplay.Commands,
		cmddogstatsdstats.Commands,
		cmdflare.Commands,
		cmdhealth.Commands,
		cmdhostname.Commands,
		cmdimport.Commands,
		cmdlaunchgui.Commands,
		cmdremoteconfig.Commands,
		cmdrun.Commands,
		cmdsecret.Commands,
		cmdsnmp.Commands,
		cmdstatus.Commands,
		cmdstreamlogs.Commands,
		cmdtaggerlist.Commands,
		cmdversion.Commands,
		cmdworkloadlist.Commands,
		cmdjmx.Commands,
		cmdsecrethelper.Commands,
		cmdintegrations.Commands,
		cmdstop.Commands,
		cmdcontrolsvc.Commands,
		cmdclosedsourceconsent.Commands,
	}
}
