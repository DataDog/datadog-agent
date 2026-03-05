// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main implements the safe-shell standalone binary.
// It runs shell scripts through the restricted interpreter with no host
// command execution, suitable for OS-level sandboxing via SysProcAttr.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

func main() {
	os.Exit(run())
}

func run() int {
	cflag := flag.String("c", "", "script to execute")
	flag.Parse()

	script, err := resolveScript(*cflag, flag.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "safe-shell: %s\n", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	r := interp.New(
		interp.WithStdout(os.Stdout),
		interp.WithStderr(os.Stderr),
		interp.WithStdin(os.Stdin),
		interp.WithEnv(os.Environ()),
	)

	if err := r.Run(ctx, script); err != nil {
		fmt.Fprintf(os.Stderr, "safe-shell: %s\n", err)
		return 2
	}

	return r.ExitCode()
}

// resolveScript determines the script source: -c flag, file argument, or stdin.
func resolveScript(cflag string, args []string) (string, error) {
	if cflag != "" {
		return cflag, nil
	}

	if len(args) > 0 {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return "", fmt.Errorf("cannot read script file: %w", err)
		}
		return string(data), nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("cannot read stdin: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("no script provided (use -c, file argument, or stdin)")
	}
	return string(data), nil
}
