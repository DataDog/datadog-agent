// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pidfile implements functions to interact with the pid file
package pidfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// WritePID writes the current PID to a file, ensuring that the file
// doesn't exist or doesn't contain a PID for a running process.
func WritePID(pidFilePath string) error {
	// check whether the pidfile exists and contains the PID for a running proc...
	if byteContent, err := os.ReadFile(pidFilePath); err == nil {
		pidStr := strings.TrimSpace(string(byteContent))
		pid, err := strconv.Atoi(pidStr)
		if err == nil && isProcess(pid) {
			// ...and return an error in case
			return fmt.Errorf("Pidfile already exists, please check %s isn't running or remove %s",
				os.Args[0], pidFilePath)
		}
	}

	// create the full path to the pidfile
	if err := os.MkdirAll(filepath.Dir(pidFilePath), os.FileMode(0755)); err != nil {
		return err
	}

	// write current pid in it
	pidStr := fmt.Sprintf("%d", os.Getpid())
	if err := os.WriteFile(pidFilePath, []byte(pidStr), 0644); err != nil {
		return err
	}

	// all good
	return nil
}
