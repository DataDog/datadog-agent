// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !secrets

// Package secrethelper implements 'agent secret-helper'
package secrethelper

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
)

// Commands returns nil when compiling without the secrets build flag
func Commands(*command.GlobalParams) []*cobra.Command {
	return nil
}
