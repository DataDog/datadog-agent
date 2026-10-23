// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command scrub runs a command, masking API and APP keys in its standard output:
//
//	scrub <command> [<args>...]
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"syscall"
)

// keys mask API and APP keys like tasks/tools/gotest-scrubbed.sh, keeping their last 5 characters,
// and unlike pkg/util/scrubber, without adding quotes that would break JSON.
var keys = []struct {
	regex *regexp.Regexp
	mask  string
}{
	{regexp.MustCompile(`\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b`), "**************************${1}"},
	{regexp.MustCompile(`\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b`), "************************************${1}"},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: scrub <command> [<args>...]")
		os.Exit(2)
	}
	cmd := exec.Command(os.Args[1], os.Args[2:]...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fail(err)
	}
	if err := cmd.Start(); err != nil {
		fail(err)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		for sig := range signals {
			_ = cmd.Process.Signal(sig) // unsupported on Windows, whose console delivers interrupts to the whole group
		}
	}()

	scrub(stdout, os.Stdout) // before cmd.Wait, which closes the pipe

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			os.Exit(exitErr.ExitCode())
		}
		fail(err)
	}
}

func scrub(in io.Reader, out io.Writer) {
	reader := bufio.NewReader(in) // unlike bufio.Scanner, not limited in line length
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			for _, key := range keys {
				line = key.regex.ReplaceAllString(line, key.mask)
			}
			if _, err := io.WriteString(out, line); err != nil {
				fail(err)
			}
		}
		if err != nil {
			return
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "scrub:", err)
	os.Exit(1)
}
