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
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/shell/executor"
	"github.com/DataDog/datadog-agent/pkg/shell/verifier"
)

type cliParams struct {
	*command.GlobalParams
	command string
	file    string
	timeout time.Duration
	manual  bool
}

// Commands returns the 'shell' subcommand.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	params := &cliParams{
		GlobalParams: globalParams,
	}

	shellCmd := &cobra.Command{
		Use:   "shell [--command cmd | --file script | script_file]",
		Short: "Execute shell commands in a safe sandbox",
		Long: `Execute shell commands after verifying they contain only allowed
commands and flags. Commands are parsed and validated against an allowlist
before execution via /bin/sh.`,
		RunE: func(_ *cobra.Command, args []string) error {
			return runShell(params, args)
		},
	}

	shellCmd.Flags().StringVar(&params.command, "command", "", "command string to execute")
	shellCmd.Flags().StringVar(&params.file, "file", "", "script file to execute")
	shellCmd.Flags().DurationVar(&params.timeout, "timeout", executor.DefaultTimeout, "execution timeout")
	shellCmd.Flags().BoolVar(&params.manual, "manual", false, "print the safe shell manual (allowed commands, features, limits)")

	return []*cobra.Command{shellCmd}
}

func runShell(params *cliParams, args []string) error {
	if params.manual {
		printManual()
		return nil
	}

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

func printManual() {
	fmt.Println("Datadog Agent Safe Shell — Manual")
	fmt.Println()
	fmt.Println("Use 'man <command>' for detailed flag documentation on this host.")
	fmt.Println()

	commands := verifier.AllowedCommandsWithDescriptions()

	// Sort command names for stable output.
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println("ALLOWED COMMANDS:")
	fmt.Println()
	for _, name := range names {
		info := commands[name]
		if info.Description != "" {
			fmt.Printf("  %s - %s\n", name, info.Description)
		} else {
			fmt.Printf("  %s\n", name)
		}

		// Sort flags for stable output.
		flags := make([]string, 0, len(info.Flags))
		for f := range info.Flags {
			flags = append(flags, f)
		}
		sort.Strings(flags)

		for _, f := range flags {
			if desc := info.Flags[f]; desc != "" {
				fmt.Printf("    %-20s %s\n", f, desc)
			} else {
				fmt.Printf("    %s\n", f)
			}
		}
		if len(flags) > 0 {
			fmt.Println()
		}
	}

	fmt.Println("ALLOWED SHELL FEATURES:")
	fmt.Println("  Pipes (|, &&, ||), for/while/until loops, if/elif/else, case statements,")
	fmt.Println("  variable assignment, parameter expansion ($VAR, ${VAR:-default}),")
	fmt.Println("  arithmetic expansion ($((expr))), block commands ({ ...; })")
	fmt.Println()

	fmt.Println("BLOCKED:")
	fmt.Printf("  Builtins: %s\n", strings.Join(verifier.BlockedBuiltins(), ", "))
	fmt.Println("  Redirections (>, >>, <, heredocs), command substitution ($(cmd), backticks),")
	fmt.Println("  process substitution, subshells, function declarations, background (&), coprocesses")
	fmt.Println()

	fmt.Printf("DANGEROUS ENV VARS (blocked in prefix assignments): %s\n",
		strings.Join(verifier.DangerousEnvVars(), ", "))
	fmt.Println()

	fmt.Println("LIMITS:")
	fmt.Printf("  Default timeout: %s | Max output: %d bytes\n",
		executor.DefaultTimeout, executor.DefaultMaxOutputBytes)
}
