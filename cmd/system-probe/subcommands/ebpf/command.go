// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ebpf provides debugging and diagnostic commands for eBPF objects.
package ebpf

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
)

// Commands returns a slice containing the eBPF top-level command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	// Add ebpf command if available on this platform
	if ebpfCmd := makeEbpfCommand(globalParams); ebpfCmd != nil {
		return []*cobra.Command{ebpfCmd}
	}

	return nil
}
