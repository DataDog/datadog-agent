// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runcmd provides support for running Cobra commands in main functions.
package runcmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"go.uber.org/dig"
)

// Run executes a cobra command and handles the results.  It is intended
// for use in `main` functions, supplying the necessary error-handling and
// exiting the process with an appropriate status.
//
// This function returns the appropriate exit status (0 or -1).
func Run(cmd *cobra.Command) int {
	// always silence errors, since they are handled here
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err != nil {
		displayError(err, cmd.ErrOrStderr())
		return -1
	}
	return 0
}

// displayError handles displaying errors from the running command.  Typically
// these are simply printed with an "Error: " prefix, but some kinds of errors
// are first simplified to reduce user confusion.
func displayError(err error, w io.Writer) {
	// RootCause returns the error it was given if it cannot find a "root cause",
	// and otherwise returns the root cause, which is more useful to the user.
	if rc := dig.RootCause(err); rc != err {
		fmt.Fprintln(w, "Error:", rc.Error())
		return
	}
	fmt.Fprintln(w, "Error:", err.Error())
}
