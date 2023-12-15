// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main for ddtray
package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/systray/command"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func main() {
	exitcode := 0
	defer func() {
		log.Flush()
		os.Exit(exitcode)
	}()
	exitcode = runcmd.Run(command.MakeCommand())
}
