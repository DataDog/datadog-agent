// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

// Package controlsvc implements 'trace-agent start-service', 'trace-agent stopservice',
// and 'trace-agent restart-service'.
package controlsvc

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
)

// Commands returns nil on Unix.
func Commands(_ func() *subcommands.GlobalParams) []*cobra.Command {
	return nil
}
