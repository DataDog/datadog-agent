// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package main

import (
	"context"
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
	port := config.Datadog.GetInt("dogstatsd_stats_port")
	s := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: http.DefaultServeMux,
	}
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			log.Errorf("Error shutting down dogstatsd stats server on port %d: %s", port, err)
		}
	}()
	go func() {
		err := s.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("Error creating dogstatsd stats server on port %v: %v", port, err)
		}
	}()

	if err := dogstatsdCmd.Execute(); err != nil {
		log.Error(err)
		// os.Exit() must be called last because it terminates immediately;
		// deferred functions are not run. Deferred function calls are executed
		// in Last In First Out order after the surrounding function returns.
		defer os.Exit(-1)
	}
}
