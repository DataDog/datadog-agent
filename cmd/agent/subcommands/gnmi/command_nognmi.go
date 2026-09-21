// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !gnmi

// Package gnmi implements the 'agent gnmi' troubleshooting subcommands
// (stub implementation, for agent flavors built without the gnmi tag).
package gnmi

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
)

// Commands returns nil when compiling without the gnmi build flag.
func Commands(*command.GlobalParams) []*cobra.Command {
	return nil
}
