// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package app

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

func initConfig(w io.Writer, cmd *cobra.Command) error {
	err := config.LoadConfigIfExists(cmd.Flag(flags.CfgPath).Value.String())
	if err != nil {
		writeError(w, err)
		return err
	}

	err = ddconfig.SetupLogger(
		"process",
		ddconfig.Datadog.GetString("log_level"),
		"",
		"",
		false,
		true,
		false,
	)
	if err != nil {
		writeError(w, err)
	}
	return err
}
