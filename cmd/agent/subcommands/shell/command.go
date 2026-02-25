// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package shell implements 'agent shell'.
package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	var (
		commandFlag     string
		allowedCommands string
	)

	shellCmd := &cobra.Command{
		Use:   "shell [script-file ...]",
		Short: "Run an embedded POSIX shell",
		Long:  `Run an embedded POSIX shell interpreter. Supports interactive mode, command strings via -c, script files, and piped stdin.`,
		RunE: func(_ *cobra.Command, args []string) error {
			opts := []interp.RunnerOption{
				interp.Interactive(true),
				interp.StdIO(os.Stdin, os.Stdout, os.Stderr),
				interp.OpenHandler(interp.SafeOpenHandler()),
			}
			if allowedCommands != "" {
				cmds := strings.Split(allowedCommands, ",")
				opts = append(opts, interp.AllowedCommands(cmds))
			}

			r, err := interp.New(opts...)
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
	shellCmd.Flags().StringVar(&allowedCommands, "allowed-commands", "", "comma-separated list of allowed external commands")

	return []*cobra.Command{shellCmd}
}

func runAll(r *interp.Runner, commandStr string, args []string) error {
	if commandStr != "" {
		return run(r, strings.NewReader(commandStr), "")
	}
	if len(args) == 0 {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return runInteractive(r, os.Stdin, os.Stdout, os.Stderr)
		}
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
	ctx := context.Background()
	return r.Run(ctx, prog)
}

func runPath(r *interp.Runner, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return run(r, f, path)
}

func runInteractive(r *interp.Runner, stdin io.Reader, stdout, stderr io.Writer) error {
	parser := syntax.NewParser()
	fmt.Fprintf(stdout, "$ ")
	var runErr error
	fn := func(stmts []*syntax.Stmt) bool {
		if parser.Incomplete() {
			fmt.Fprintf(stdout, "> ")
			return true
		}
		ctx := context.Background()
		for _, stmt := range stmts {
			if err := r.Run(ctx, stmt); err != nil {
				var es interp.ExitStatus
				if errors.As(err, &es) {
					fmt.Fprintf(stderr, "exit status %d\n", int(es))
				} else {
					fmt.Fprintln(stderr, err)
				}
				if r.Exited() {
					runErr = err
					return false
				}
			}
		}
		fmt.Fprintf(stdout, "$ ")
		return true
	}
	if err := parser.Interactive(stdin, fn); err != nil {
		return err
	}
	return runErr
}
