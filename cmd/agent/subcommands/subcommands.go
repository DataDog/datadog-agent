// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/agent/command"
	cmdcheck "github.com/DataDog/datadog-agent/cmd/agent/subcommands/check"
	cmdconfig "github.com/DataDog/datadog-agent/cmd/agent/subcommands/config"
	cmdconfigcheck "github.com/DataDog/datadog-agent/cmd/agent/subcommands/configcheck"
	cmddiagnose "github.com/DataDog/datadog-agent/cmd/agent/subcommands/diagnose"
	cmddogstatsdcapture "github.com/DataDog/datadog-agent/cmd/agent/subcommands/dogstatsdcapture"
	cmddogstatsdreplay "github.com/DataDog/datadog-agent/cmd/agent/subcommands/dogstatsdreplay"
	cmddogstatsdstats "github.com/DataDog/datadog-agent/cmd/agent/subcommands/dogstatsdstats"
	cmdflare "github.com/DataDog/datadog-agent/cmd/agent/subcommands/flare"
	cmdhealth "github.com/DataDog/datadog-agent/cmd/agent/subcommands/health"
	cmdhostname "github.com/DataDog/datadog-agent/cmd/agent/subcommands/hostname"
	cmdimport "github.com/DataDog/datadog-agent/cmd/agent/subcommands/import"
	cmdlaunchgui "github.com/DataDog/datadog-agent/cmd/agent/subcommands/launchgui"
	cmdremoteconfig "github.com/DataDog/datadog-agent/cmd/agent/subcommands/remoteconfig"
	cmdrun "github.com/DataDog/datadog-agent/cmd/agent/subcommands/run"
	cmdsecret "github.com/DataDog/datadog-agent/cmd/agent/subcommands/secret"
	cmdsnmp "github.com/DataDog/datadog-agent/cmd/agent/subcommands/snmp"
	cmdstatus "github.com/DataDog/datadog-agent/cmd/agent/subcommands/status"
	cmdstreamlogs "github.com/DataDog/datadog-agent/cmd/agent/subcommands/streamlogs"
	cmdtaggerlist "github.com/DataDog/datadog-agent/cmd/agent/subcommands/taggerlist"
	cmdversion "github.com/DataDog/datadog-agent/cmd/agent/subcommands/version"
	cmdworkloadlist "github.com/DataDog/datadog-agent/cmd/agent/subcommands/workloadlist"
)

// AgentSubcommands returns SubcommandFactories for the subcommands supported
// with the current build flags.
func AgentSubcommands() []command.SubcommandFactory {
	factories := []command.SubcommandFactory{
		// universal subcommands, present for all build flag combinations
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
	}
	factories = append(factories, secretsSubcommands()...)
	factories = append(factories, pythonSubcommands()...)
	factories = append(factories, jmxSubcommands()...)
	factories = append(factories, windowsSubcommands()...)

	return factories
}
