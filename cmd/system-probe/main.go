// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/system-probe/app"
)

func main() {
	setDefaultCommandIfNonePresent()
	checkForDeprecatedFlags()
	if err := app.SysprobeCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
