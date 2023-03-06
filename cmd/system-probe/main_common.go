// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows
// +build linux windows

package main

import (
	"os"

	"github.com/spf13/cobra"
)

func setDefaultCommandIfNonePresent(rootCmd *cobra.Command) {
	var subCommandNames []string
	for _, command := range rootCmd.Commands() {
		subCommandNames = append(subCommandNames, append(command.Aliases, command.Name())...)
	}

	args := []string{os.Args[0], "run"}
	if len(os.Args) > 1 {
		potentialCommand := os.Args[1]
		if potentialCommand == "help" {
			return
		}

		for _, command := range subCommandNames {
			if command == potentialCommand {
				return
			}
		}
		args = append(args, os.Args[1:]...)
	}
	os.Args = args
}
