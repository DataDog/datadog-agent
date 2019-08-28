// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows
// +build kubeapiserver

//go:generate go run ../../pkg/config/render_config.go dca ../../pkg/config/config_template.yaml ../../Dockerfiles/cluster-agent/datadog-cluster.yaml

package main

import (
	"fmt"
	"net/http"
	"os"

	_ "expvar"         // Blank import used because this isn't directly used in this file
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"github.com/prometheus/client_golang/prometheus/promhttp"

	_ "github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster"
	_ "github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/kubeapi"
	_ "github.com/StackVista/stackstate-agent/pkg/collector/corechecks/net"
	_ "github.com/StackVista/stackstate-agent/pkg/collector/corechecks/system"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/util/log"

	"github.com/StackVista/stackstate-agent/cmd/cluster-agent/app"
)

func main() {
	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", config.Datadog.GetInt("metrics_port")), nil)

	if err := app.ClusterAgentCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
