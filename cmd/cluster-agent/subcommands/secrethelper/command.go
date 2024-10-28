// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package secrethelper implements 'cluster-agent secret-helper'.
//
//nolint:revive // TODO(CINT) Fix revive linter
package diagnose

import (
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/cmd/secrethelper"
	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
//
//nolint:revive // TODO(CINT) Fix revive linter
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	return secrethelper.Commands()
}
