// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux_bpf

package usm

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
)

// makeCheckMapsCommand returns nil on platforms without eBPF support
func makeCheckMapsCommand(_ *command.GlobalParams) *cobra.Command {
	return nil
}
