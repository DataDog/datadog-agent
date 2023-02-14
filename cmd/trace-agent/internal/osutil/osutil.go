// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package osutil

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/internal/flags"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// Exists reports whether the given path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// Exit prints the message and exits the program with status code 1.
func Exit(msg string) {
	if toStdOut() {
		fmt.Println(msg)
	} else {
		log.Error(msg)
		log.Flush()
	}
	os.Exit(1)
}

// Exitf prints the formatted text and exits the program with status code 1.
func Exitf(format string, args ...interface{}) {
	if toStdOut() {
		fmt.Printf(format, args...)
		fmt.Println("")
	} else {
		log.Criticalf(format, args...)
		log.Flush()
	}
	os.Exit(1)
}

// toStdOut returns whether errors should be written to stdout.
// Returns true if the agent's info or version commands are invoked,
// or if the logger hasn't been set up yet (typically on startup).
func toStdOut() bool {
	return flags.Info || flags.Version || !log.IsSet()
}
