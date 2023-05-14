// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package main

import (
	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
)

const useWinParams = false

func rootCmdRun(globalParams *command.GlobalParams) {
	exit := make(chan struct{})

	// Invoke the Agent
	runAgent(globalParams, exit)
}
