// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bazelbuild/rules_go/go/tools/bzltestutil"
)

type options struct {
	label      string
	logPath    string
	outputPath string
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, err := parseFlags(args)
	if err != nil {
		return err
	}

	var out io.Writer = stdout
	if opts.outputPath != "" && opts.outputPath != "-" {
		outFile, err := os.Create(opts.outputPath)
		if err != nil {
			return fmt.Errorf("create output %q: %w", opts.outputPath, err)
		}
		defer outFile.Close()
		out = outFile
	}

	if err := convert(opts.label, opts.logPath, out); err != nil {
		return err
	}

	fmt.Fprintf(stderr, "Converted %s to test2json\n", opts.logPath)
	return nil
}

func parseFlags(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("testlogs_to_json", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.label, "label", "", "Bazel test label to record as the test2json package")
	fs.StringVar(&opts.logPath, "log", "", "Path to the test.log to convert")
	fs.StringVar(&opts.outputPath, "output", "-", "Path to write test2json JSONL output, or '-' for stdout")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() != 0 {
		return opts, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if opts.label == "" || opts.logPath == "" {
		return opts, errors.New("both -label and -log are required")
	}
	return opts, nil
}

// convert streams a Bazel test.log through the rules_go test2json converter.
//
// The Bazel label, not the Go import path, is recorded as the test2json package:
// dd_agent_go_test emits several configured go_test targets for the same Go
// package under different build-tag sets, and collapsing them to one import path
// would make UTOF report those distinct runs as retries of each other.
func convert(label, logPath string, out io.Writer) error {
	f, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("open test log %q: %w", logPath, err)
	}
	defer f.Close()

	converter := bzltestutil.NewConverter(out, label, bzltestutil.Timestamp)
	if _, err := io.Copy(converter, f); err != nil {
		return fmt.Errorf("convert test log %q: %w", logPath, err)
	}
	if err := converter.Close(); err != nil {
		return fmt.Errorf("close converter for test log %q: %w", logPath, err)
	}
	return nil
}
