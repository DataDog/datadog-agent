// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	_ "net/http/pprof"
	"os"

	"log"

	"github.com/DataDog/datadog-agent/cmd/logs-agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func main() {
	flavor.SetFlavor(flavor.LogsAgent)

	if err := command.MakeRootCommand().Execute(); err != nil {
		log.Println(err)
		os.Exit(-1)
	}
}
