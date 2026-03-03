// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package shell implements 'agent shell'.
package shell

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	var commandFlag string

	shellCmd := &cobra.Command{
		Use:    "shell [script-file ...]",
		Short:  "[experimental] Run an embedded shell",
		Hidden: true,
		RunE: func(_ *cobra.Command, args []string) error {
			r, err := interp.New(
				interp.StdIO(os.Stdin, os.Stdout, os.Stderr),
			)
			if err != nil {
				return err
			}

			err = runAll(r, commandFlag, args)
			var es interp.ExitStatus
			if errors.As(err, &es) {
				os.Exit(int(es))
			}
			return err
		},
	}
	shellCmd.Flags().StringVar(&commandFlag, "command", "", "command string to execute")

	return []*cobra.Command{shellCmd}
}

func runAll(r *interp.Runner, commandStr string, args []string) error {
	if commandStr != "" {
		return run(r, strings.NewReader(commandStr), "")
	}
	if len(args) == 0 {
		return run(r, os.Stdin, "")
	}
	for _, path := range args {
		if err := runPath(r, path); err != nil {
			return err
		}
	}
	return nil
}

func run(r *interp.Runner, reader io.Reader, name string) error {
	prog, err := syntax.NewParser().Parse(reader, name)
	if err != nil {
		return err
	}
	r.Reset()
	return r.Run(context.Background(), prog)
}

func runPath(r *interp.Runner, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return run(r, f, path)
}
