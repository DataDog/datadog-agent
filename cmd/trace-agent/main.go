// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultLogFile = "/var/log/datadog/trace-agent.log"

func main() {

	if err := command.MakeRootCommand(defaultLogFile).Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
