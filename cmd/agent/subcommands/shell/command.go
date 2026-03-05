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
	"time"

	"github.com/spf13/cobra"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	var scriptFlag string
	var allowedPathsFlag string
	var timeoutFlag time.Duration

	shellCmd := &cobra.Command{
		Use:    "shell",
		Short:  "[experimental] Run an embedded shell",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			opts := []interp.RunnerOption{
				interp.StdIO(os.Stdin, os.Stdout, os.Stderr),
			}
			if allowedPathsFlag != "" {
				paths := strings.Split(allowedPathsFlag, ",")
				opts = append(opts, interp.AllowedPaths(paths))
			}
			r, err := interp.New(opts...)
			if err != nil {
				return err
			}
			defer r.Close()

			var reader io.Reader = os.Stdin
			if scriptFlag != "" {
				reader = strings.NewReader(scriptFlag)
			}
			err = run(r, reader, timeoutFlag)
			var es interp.ExitStatus
			if errors.As(err, &es) {
				os.Exit(int(es))
			}
			return err
		},
	}
	shellCmd.Flags().StringVar(&scriptFlag, "script", "", "script string to execute")
	shellCmd.Flags().StringVar(&allowedPathsFlag, "allowed-paths", "", "comma-separated list of directories to restrict file access to")
	shellCmd.Flags().DurationVar(&timeoutFlag, "timeout", 60*time.Second, "maximum execution time for the shell script (0 for no timeout)")

	return []*cobra.Command{shellCmd}
}

func run(r *interp.Runner, reader io.Reader, timeout time.Duration) error {
	prog, err := syntax.NewParser().Parse(reader, "")
	if err != nil {
		return err
	}
	r.Reset()
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return r.Run(ctx, prog)
}
