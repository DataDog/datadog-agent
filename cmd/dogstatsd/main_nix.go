// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !windows

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultLogFile = "/var/log/datadog/dogstatsd.log"

func main() {
	flavor.SetFlavor(flavor.Dogstatsd)

	// go_expvar server
	go func() {
		port := config.Datadog.GetInt("dogstatsd_stats_port")
		err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), http.DefaultServeMux)
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("Error creating expvar server on port %v: %v", port, err)
		}
	}()

	if err := dogstatsdCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
