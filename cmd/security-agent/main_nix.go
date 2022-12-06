// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package main

import (
	_ "expvar"         // Blank import used because this isn't directly used in this file
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file
	"os"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func main() {
	// set the Agent flavor
	flavor.SetFlavor(flavor.SecurityAgent)

	if err := app.CreateSecurityAgentCmd().Execute(); err != nil {
		os.Exit(-1)
	}
}
