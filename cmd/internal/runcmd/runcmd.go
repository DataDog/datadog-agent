// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runcmd provides support for running Cobra commands in main functions.
package runcmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Run executes a cobra command and handles the results.  It is intended
// for use in `main` functions, supplying the necessary error-handling and
// exiting the process with an appropriate status.
//
// This function does not return.
func Run(cmd *cobra.Command) {
	// always silence errors, since they are handled here
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Error:", err.Error())
		os.Exit(-1)
	}
	os.Exit(0)
}
