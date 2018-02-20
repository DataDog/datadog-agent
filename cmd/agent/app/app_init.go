// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package app

/*
Package app implements the Agent main loop, orchestrating
all the components and providing the command line interface. */

import (
	"flag"
)

func init() {
	AgentCmd.PersistentFlags().StringVarP(&confFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	AgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")

	flag.Lookup("stderrthreshold").Value.Set("FATAL")
	// Convinces goflags that we have called Parse() to avoid noisy logs.
	// OSS Issue: kubernetes/kubernetes#17162.
	flag.CommandLine.Parse([]string{})
}
