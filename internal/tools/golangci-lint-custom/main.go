// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/golangci/golangci-lint/v2/pkg/commands"
	"github.com/golangci/golangci-lint/v2/pkg/exitcodes"

	_ "github.com/DataDog/datadog-agent/internal/tools/regexphoist"
)

func main() {
	// Hash the custom binary for cache invalidation when a plugin changes.
	info := commands.BuildInfo{
		Version:   "(devel)",
		GoVersion: runtime.Version(),
	}
	if err := commands.Execute(info); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "The command is terminated due to an error: %v\n", err)
		os.Exit(exitcodes.Failure)
	}
}
