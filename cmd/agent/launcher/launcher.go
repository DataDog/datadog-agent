// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Main package for the launcher binary
package main

import (
	"fmt"
	"os"
	"path"
	"syscall"
)

// EmbeddedPath stores the omnibus embedded path
var EmbeddedPath string

func main() {
	agent := os.Getenv("DD_BUNDLED_AGENT")
	if agent == "" {
		agent = "agent"
	}

	if _, err := os.Stat(EmbeddedPath); EmbeddedPath == "" || err != nil {
		executablePath, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to location Datadog Agent location: %s\n", err)
			os.Exit(1)
		}
		EmbeddedPath = path.Join(path.Dir(executablePath), "..")
	}

	mainAgentPath := path.Clean(path.Join(EmbeddedPath, "..", "bin", "agent", "agent"))

	argv := []string{agent}
	if len(os.Args) > 1 {
		argv = append(argv, os.Args[1:]...)
	}

	if err := syscall.Exec(mainAgentPath, argv, os.Environ()); err != nil {
		if err, ok := err.(syscall.Errno); ok {
			fmt.Fprintf(os.Stderr, "Failed to execute Datadog Agent at %s: %s\n", mainAgentPath, err)
			os.Exit(int(err))
		}

		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
