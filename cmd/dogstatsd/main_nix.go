// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultLogFile = "/var/log/datadog/dogstatsd.log"

func main() {
	// go_expvar server
	go http.ListenAndServe( //nolint:errcheck
		fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_stats_port")),
		http.DefaultServeMux)

	if err := dogstatsdCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
