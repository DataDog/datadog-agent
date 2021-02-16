// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !android
// +build !windows,!android

package main

import (
	"os"
	"strings"

	agent "github.com/DataDog/datadog-agent/cmd/agent/app"
	process "github.com/DataDog/datadog-agent/cmd/process-agent/app"
	security "github.com/DataDog/datadog-agent/cmd/security-agent/app"
	trace "github.com/DataDog/datadog-agent/cmd/trace-agent/app"
)

func main() {
	switch {
	case strings.Contains(os.Args[0], "process-agent"):
		process.Run()
	case strings.Contains(os.Args[0], "trace-agent"):
		trace.Run()
	case strings.Contains(os.Args[0], "security-agent"):
		security.Run()
	default:
		agent.Run()
	}
}
