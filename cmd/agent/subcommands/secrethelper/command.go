// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets
// +build secrets

// Package secrethelper implements 'agent secret-helper'
package secrethelper

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/secrets"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalArgs *command.GlobalArgs) []*cobra.Command {
	// TODO: move to cmd/common/secrethelper?
	return []*cobra.Command{secrets.SecretHelperCmd}
}
