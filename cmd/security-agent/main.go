// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package main

import (
	"fmt"
	"net/http"
	"os"

	_ "expvar"         // Blank import used because this isn't directly used in this file
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app"
)

func main() {
	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", telemetry.Handler())
	go http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", config.Datadog.GetInt("metrics_port")), nil) //nolint:errcheck

	if err := app.SecurityAgentCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
