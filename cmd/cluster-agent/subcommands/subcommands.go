// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	cmdcheck "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/check"
	cmdclusterchecks "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/clusterchecks"
	cmdcompliance "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/compliance"
	cmdconfig "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/config"
	cmdconfigcheck "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/configcheck"
	cmddiagnose "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/diagnose"
	cmdflare "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/flare"
	cmdhealth "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/health"
	cmdmetamap "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/metamap"
	cmdsecrethelper "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/secrethelper"
	cmdstart "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/start"
	cmdstatus "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/status"
	cmdtaggerlist "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/taggerlist"
	cmdtelemetry "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/telemetry"
	cmdversion "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/version"
	cmdworkloadlist "github.com/DataDog/datadog-agent/cmd/cluster-agent/subcommands/workloadlist"
)

// ClusterAgentSubcommands returns SubcommandFactories for the subcommands
// supported with the current build flags.
func ClusterAgentSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		cmdstart.Commands,
		cmdversion.Commands,
		cmdcheck.Commands,
		cmdconfig.Commands,
		cmdhealth.Commands,
		cmdconfigcheck.Commands,
		cmdclusterchecks.Commands,
		cmdcompliance.Commands,
		cmdflare.Commands,
		cmddiagnose.Commands,
		cmdmetamap.Commands,
		cmdsecrethelper.Commands,
		cmdtelemetry.Commands,
		cmdstatus.Commands,
		cmdworkloadlist.Commands,
		cmdtaggerlist.Commands,
	}
}
