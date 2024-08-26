// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package check implements 'cluster-agent check'.
package check

import (
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/check"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	pkgcommon "github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	ctx, _ := pkgcommon.GetMainCtxCancel()
	// Create the Leader election engine without initializing it
	if pkgconfig.Datadog().GetBool("leader_election") {
		leaderelection.CreateGlobalLeaderEngine(ctx)
	}

	cmd := check.MakeCommand(func() check.GlobalParams {
		return check.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
			ConfigName:   command.ConfigName,
			LoggerName:   command.LoggerName,
		}
	})

	return []*cobra.Command{cmd}
}
