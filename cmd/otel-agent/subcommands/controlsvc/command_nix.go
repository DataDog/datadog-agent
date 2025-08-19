//go:build !windows && otlp

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package controlsvc implements 'otel-agent start-service', 'otel-agent stop-service',
// and 'otel-agent restart-service'.
package controlsvc

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
)

// Commands returns nil on non-Windows systems.
func Commands(_ *subcommands.GlobalParams) []*cobra.Command {
	return nil
}
