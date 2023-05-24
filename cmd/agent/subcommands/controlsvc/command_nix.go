// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package controlsvc implements 'agent start-service', 'agent stopservice',
// and 'agent restart-service'.
package controlsvc

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
)

// Commands returns nil on Unix.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	return nil
}
