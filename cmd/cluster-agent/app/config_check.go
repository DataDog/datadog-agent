// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package app

import "github.com/DataDog/datadog-agent/pkg/util/clusteragent/commands"

func init() {
	ClusterAgentCmd.AddCommand(commands.GetConfigCheckCobraCmd(&flagNoColor, &confPath, loggerName))
}
