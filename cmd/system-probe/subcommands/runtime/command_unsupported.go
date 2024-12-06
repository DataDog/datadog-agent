// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

// Package runtime holds runtime related files
package runtime

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
)

// Commands returns the runtime security commands
func Commands(*command.GlobalParams) []*cobra.Command {
	return nil
}
