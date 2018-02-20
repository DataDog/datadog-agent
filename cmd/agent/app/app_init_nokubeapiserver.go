// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !kubeapiserver

package app

/*
Package app implements the Agent main loop, orchestrating
all the components and providing the command line interface. */

// app_init_nokubeapiserver is for the puppy agent.
// As the buildflag kubeapiserver is not used there is no flag `stderrthreshold` to set.
func init() {
	AgentCmd.PersistentFlags().StringVarP(&confFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	AgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
}
