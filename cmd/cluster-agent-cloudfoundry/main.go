// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows
// +build clusterchecks

//go:generate go run ../../pkg/config/render_config.go dcacf ../../pkg/config/config_template.yaml ../../cloudfoundry.yaml

package main

import (
	"os"

	_ "expvar"         // Blank import used because this isn't directly used in this file
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"github.com/DataDog/datadog-agent/cmd/cluster-agent-cloudfoundry/app"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func main() {
	var returnCode int
	if err := app.ClusterAgentCmd.Execute(); err != nil {
		log.Error(err)
		returnCode = -1
	}
	log.Flush()
	os.Exit(returnCode)
}
