// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/cmd/dogstatsd/command"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultLogFile = "/var/log/datadog/dogstatsd.log"

func main() {
	flavor.SetFlavor(flavor.Dogstatsd)

	if err := command.MakeRootCommand(defaultLogFile).Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
