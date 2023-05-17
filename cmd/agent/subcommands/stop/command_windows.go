// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package stop implements 'agent stop'.
package stop

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
)

// Commands returns nils on windows where the agent is run as a Windows service.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	return nil
}
