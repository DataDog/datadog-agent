// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

//go:generate go run ../../pkg/config/render_config.go dogstatsd ../../pkg/config/config_template.yaml ./dist/dogstatsd.yaml

package main

import (
	_ "expvar"
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/cmd/dogstatsd/app"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func main() {
	if err := app.DogstatsdCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
