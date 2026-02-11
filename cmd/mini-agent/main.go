// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// mini-agent is a standalone binary that provides minimal Datadog Agent functionality
// including tagger server and basic metric submission.
package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/mini-agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	_ "github.com/DataDog/datadog-agent/pkg/version"
)

func main() {
	flavor.SetFlavor(flavor.DefaultAgent)
	os.Exit(runcmd.Run(command.MakeRootCommand()))
}
