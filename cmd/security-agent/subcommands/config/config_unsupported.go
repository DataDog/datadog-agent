// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

//nolint:revive // TODO(SEC) Fix revive linter
package config

import (
	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/spf13/cobra"
)

//nolint:revive // TODO(SEC) Fix revive linter
func Commands(*command.GlobalParams) []*cobra.Command {
	return nil
}
