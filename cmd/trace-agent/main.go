// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/command"
)

func main() {
	os.Args = command.FixDeprecatedFlags(os.Args, os.Stdout)

	os.Exit(runcmd.Run(command.MakeRootCommand()))
}
