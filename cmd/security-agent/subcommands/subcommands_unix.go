// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package subcommands implement security agent subcommands
package subcommands

import (
	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	cmdcompliance "github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/compliance"
)

func init() {
	SecurityAgentSubcommands = append(SecurityAgentSubcommands,
		command.SubcommandFactoryFromOne(cmdcompliance.CheckCommand), // maintained as legacy "security-agent check", mapped to "security-agent compliance check"
		cmdcompliance.Commands,
	)
}
