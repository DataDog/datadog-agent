// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package subcommands holds the subcommands for the private-action-runner command
package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/subcommands/run"
	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/subcommands/version"
)

// PrivateActionRunnerSubcommands returns all subcommands for the private-action-runner command
func PrivateActionRunnerSubcommands() []command.SubcommandFactory {
	return []command.SubcommandFactory{
		run.Commands,
		version.Commands,
	}
}
