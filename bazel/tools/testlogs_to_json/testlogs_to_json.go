// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bazelbuild/rules_go/go/tools/bzltestutil"
)

type manifestEntry struct {
	pkg     string
	logPath string
}

type options struct {
	manifestPath string
	outputPath   string
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

	entries, err := readManifest(opts.manifestPath)
	if err != nil {
		return err
	}

	var out io.Writer = stdout
	var outFile *os.File
	if opts.outputPath != "" && opts.outputPath != "-" {
		outFile, err = os.Create(opts.outputPath)
		if err != nil {
			return fmt.Errorf("create output %q: %w", opts.outputPath, err)
		}
		defer outFile.Close()
		out = outFile
	}

	if err := convert(entries, out); err != nil {
		return err
	}

	fmt.Fprintf(stderr, "Converted %d Bazel test logs to test2json\n", len(entries))
	return nil
}

func parseFlags(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("testlogs_to_json", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.manifestPath, "manifest", "", "Path to a tab-separated manifest: <go import path>\\t<test.log path>")
	fs.StringVar(&opts.outputPath, "output", "-", "Path to write test2json JSONL output, or '-' for stdout")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.manifestPath == "" {
		return opts, errors.New("missing required -manifest")
	}
	if fs.NArg() != 0 {
		return opts, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return opts, nil
}

func readManifest(path string) ([]manifestEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open manifest %q: %w", path, err)
	}
	defer f.Close()

	var entries []manifestEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		pkg, logPath, ok := strings.Cut(line, "\t")
		if !ok || pkg == "" || logPath == "" {
			return nil, fmt.Errorf("invalid manifest line: expected <go import path>\\t<test.log path>, got %q", line)
		}
		if strings.Contains(logPath, "\t") {
			return nil, fmt.Errorf("invalid manifest line: too many tab-separated fields, got %q", line)
		}
		entries = append(entries, manifestEntry{pkg: pkg, logPath: logPath})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", path, err)
	}
	return entries, nil
}

func convert(entries []manifestEntry, out io.Writer) error {
	for _, entry := range entries {
		f, err := os.Open(entry.logPath)
		if err != nil {
			return fmt.Errorf("open test log %q for package %s: %w", entry.logPath, entry.pkg, err)
		}

		converter := bzltestutil.NewConverter(out, entry.pkg, bzltestutil.Timestamp)
		_, copyErr := io.Copy(converter, f)
		closeErr := f.Close()
		if copyErr != nil {
			return fmt.Errorf("convert test log %q for package %s: %w", entry.logPath, entry.pkg, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close test log %q for package %s: %w", entry.logPath, entry.pkg, closeErr)
		}
		if err := converter.Close(); err != nil {
			return fmt.Errorf("close converter for test log %q package %s: %w", entry.logPath, entry.pkg, err)
		}
	}
	return nil
}
