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
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
)

type runConfig struct {
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	stdinFile *os.File
}

func (c runConfig) isTerminal() bool {
	if c.stdinFile == nil {
		return false
	}
	return term.IsTerminal(int(c.stdinFile.Fd()))
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	var commandStr string

	shellCmd := &cobra.Command{
		Use:   "shell [path ...]",
		Short: "Run a POSIX shell",
		Args:  cobra.ArbitraryArgs,
		Run: func(_ *cobra.Command, args []string) {
			cfg := runConfig{
				stdin:     os.Stdin,
				stdout:    os.Stdout,
				stderr:    os.Stderr,
				stdinFile: os.Stdin,
			}
			os.Exit(runShell(commandStr, args, cfg))
		},
	}

	shellCmd.Flags().StringVarP(&commandStr, "command", "c", "", "command to be executed")

	return []*cobra.Command{shellCmd}
}

func runShell(commandStr string, paths []string, cfg runConfig) int {
	err := runAll(commandStr, paths, cfg)
	var exitStatus interp.ExitStatus
	if errors.As(err, &exitStatus) {
		return int(exitStatus)
	}
	if err != nil {
		fmt.Fprintln(cfg.stderr, err)
		return 1
	}
	return 0
}

func runAll(commandStr string, paths []string, cfg runConfig) error {
	r, err := interp.New(
		interp.Interactive(true),
		interp.StdIO(cfg.stdin, cfg.stdout, cfg.stderr),
	)
	if err != nil {
		return err
	}

	if commandStr != "" {
		return run(r, strings.NewReader(commandStr), "")
	}

	if len(paths) == 0 {
		if cfg.isTerminal() {
			return runInteractive(r, cfg.stdin, cfg.stdout, cfg.stderr)
		}
		return run(r, cfg.stdin, "")
	}

	for _, path := range paths {
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
	for stmts, err := range parser.InteractiveSeq(stdin) {
		if err != nil {
			return err // stop at the first error
		}
		if parser.Incomplete() {
			fmt.Fprintf(stdout, "> ")
			continue
		}
		ctx := context.Background()
		for _, stmt := range stmts {
			err := r.Run(ctx, stmt)
			if r.Exited() {
				return err
			}
		}
		fmt.Fprintf(stdout, "$ ")
	}
	return nil
}
