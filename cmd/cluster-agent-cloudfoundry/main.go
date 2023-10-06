// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && clusterchecks

//go:generate go run ../../pkg/config/render_config.go dcacf ../../pkg/config/config_template.yaml ../../cloudfoundry.yaml

package main

import (
	_ "expvar"         // Blank import used because this isn't directly used in this file
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file
	"os"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent-cloudfoundry/command"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent-cloudfoundry/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func main() {
	flavor.SetFlavor(flavor.ClusterAgent)

	ClusterAgentCmd := command.MakeCommand(subcommands.ClusterAgentSubcommands())

	if err := ClusterAgentCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
