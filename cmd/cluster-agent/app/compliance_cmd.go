// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver
// +build !windows,kubeapiserver

package app

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/check"
	"github.com/DataDog/datadog-agent/comp/core"
)

var (
	complianceCmd = &cobra.Command{
		Use:   "compliance",
		Short: "Compliance utility commands",
	}
)

func init() {
	// TODO: this is not yet correct, the config name is not set correctly
	bundleParams := core.BundleParams{
		ConfFilePath:      confPath,
		ConfigLoadSecrets: false,
	}.LogForOneShot(string(loggerName), "off", true)

	complianceCmd.AddCommand(check.Commands(bundleParams)...)
	ClusterAgentCmd.AddCommand(complianceCmd)
}
