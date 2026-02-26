// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package shell implements the 'agent shell' subcommand for safe shell execution.
package shell

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/shell/executor"
)

type cliParams struct {
	*command.GlobalParams
	command string
	file    string
	timeout time.Duration
}

// Commands returns the 'shell' subcommand.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	params := &cliParams{
		GlobalParams: globalParams,
	}

	shellCmd := &cobra.Command{
		Use:   "shell [-c command | script_file]",
		Short: "Execute shell commands in a safe sandbox",
		Long: `Execute shell commands after verifying they contain only allowed
commands and flags. Commands are parsed and validated against an allowlist
before execution via /bin/sh.`,
		RunE: func(_ *cobra.Command, args []string) error {
			return runShell(params, args)
		},
	}

	shellCmd.Flags().StringVarP(&params.command, "command", "c", "", "command string to execute")
	shellCmd.Flags().StringVarP(&params.file, "file", "f", "", "script file to execute")
	shellCmd.Flags().DurationVarP(&params.timeout, "timeout", "t", executor.DefaultTimeout, "execution timeout")

	return []*cobra.Command{shellCmd}
}

func runShell(params *cliParams, args []string) error {
	ctx := context.Background()
	opts := []executor.Option{
		executor.WithTimeout(params.timeout),
	}

	// Mode 1: Execute a command string (-c)
	if params.command != "" {
		return executeAndPrint(ctx, params.command, opts)
	}

	// Mode 2: Execute a script file (-f)
	if params.file != "" {
		content, err := os.ReadFile(params.file)
		if err != nil {
			return fmt.Errorf("failed to read script file: %w", err)
		}
		return executeAndPrint(ctx, string(content), opts)
	}

	// Mode 3: Script file as positional argument
	if len(args) > 0 {
		content, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("failed to read script file: %w", err)
		}
		return executeAndPrint(ctx, string(content), opts)
	}

	// Mode 4: Interactive mode (stdin)
	return runInteractive(ctx, opts)
}

// exitCodeError wraps a non-zero exit code so the caller can propagate it.
type exitCodeError struct {
	code int
}

func (e *exitCodeError) Error() string {
	return fmt.Sprintf("command exited with code %d", e.code)
}

func executeAndPrint(ctx context.Context, script string, opts []executor.Option) error {
	result, err := executor.Execute(ctx, script, opts...)
	if err != nil {
		return err
	}

	if result.Stdout != "" {
		fmt.Print(result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}

	if result.ExitCode != 0 {
		return &exitCodeError{code: result.ExitCode}
	}

	return nil
}

func runInteractive(ctx context.Context, opts []executor.Option) error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Datadog Agent Safe Shell (type 'exit' to quit)")
	fmt.Print("$ ")

	for scanner.Scan() {
		line := scanner.Text()
		if line == "exit" || line == "quit" {
			return nil
		}
		if line == "" {
			fmt.Print("$ ")
			continue
		}

		err := executeAndPrint(ctx, line, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		fmt.Print("$ ")
	}

	return scanner.Err()
}
