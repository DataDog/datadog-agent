// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/agent/app"
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
	cmdstart "github.com/DataDog/datadog-agent/cmd/agent/subcommands/start"
	cmdstatus "github.com/DataDog/datadog-agent/cmd/agent/subcommands/status"
	cmdstreamlogs "github.com/DataDog/datadog-agent/cmd/agent/subcommands/streamlogs"
	cmdtaggerlist "github.com/DataDog/datadog-agent/cmd/agent/subcommands/taggerlist"
	cmdtroubleshooting "github.com/DataDog/datadog-agent/cmd/agent/subcommands/troubleshooting"
	cmdversion "github.com/DataDog/datadog-agent/cmd/agent/subcommands/version"
	cmdworkloadlist "github.com/DataDog/datadog-agent/cmd/agent/subcommands/workloadlist"
)

// AgentSubcommands returns SubcommandFactories for the subcommands supported
// with the current build flags.
func AgentSubcommands() []app.SubcommandFactory {
	factories := []app.SubcommandFactory{
		// universal subcommands, present for all build flag combinations
		cmdcheck.Command,
		cmdconfigcheck.Command,
		cmdconfig.Command,
		cmddiagnose.Command,
		cmddogstatsdcapture.Command,
		cmddogstatsdreplay.Command,
		cmddogstatsdstats.Command,
		cmdflare.Command,
		cmdhealth.Command,
		cmdhostname.Command,
		cmdimport.Command,
		cmdlaunchgui.Command,
		cmdremoteconfig.Command,
		cmdrun.Command,
		cmdsecret.Command,
		cmdsnmp.Command,
		cmdstart.Command,
		cmdstatus.Command,
		cmdstreamlogs.Command,
		cmdtaggerlist.Command,
		cmdtroubleshooting.Command,
		cmdversion.Command,
		cmdworkloadlist.Command,
	}
	factories = append(factories, secretsSubcommands()...)
	factories = append(factories, pythonSubcommands()...)
	factories = append(factories, jmxSubcommands()...)
	factories = append(factories, windowsSubcommands()...)

	return factories
}
