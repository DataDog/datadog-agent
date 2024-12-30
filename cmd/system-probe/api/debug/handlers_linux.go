// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package debug contains handlers for debug information global to all of system-probe
package debug

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"syscall"
	"time"
)

var klogRegexp = regexp.MustCompile(`<(\d+)>(.*)`)

func readAllDmesg() ([]byte, error) {
	const syslogActionSizeBuffer = 10
	const syslogActionReadAll = 3

	n, err := syscall.Klogctl(syslogActionSizeBuffer, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query size of log buffer [%w]", err)
	}

	b := make([]byte, n)

	m, err := syscall.Klogctl(syslogActionReadAll, b)
	if err != nil {
		return nil, fmt.Errorf("failed to read messages from log buffer [%w]", err)
	}

	return b[:m], nil
}

func parseDmesg(buffer []byte) (string, error) {
	buf := bytes.NewBuffer(buffer)
	var result string

	for {
		line, err := buf.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			return result, err
		}

		parts := klogRegexp.FindStringSubmatch(line)
		if parts != nil {
			result += parts[2] + "\n"
		} else {
			result += line
		}
	}

	return result, nil
}

// HandleDmesg writes linux dmesg into the HTTP response.
func HandleDmesg(w http.ResponseWriter, _ *http.Request) {
	dmesg, err := readAllDmesg()
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "failed to read dmesg: %s", err)
		return
	}
	dmesgStr, err := parseDmesg(dmesg)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "failed to parse dmesg: %s", err)
		return
	}

	io.WriteString(w, dmesgStr)
}

// handleCommand runs commandName with the provided arguments and writes it to the HTTP response.
// If the command exits with a failure or doesn't exist in the PATH, it will still 200 but report the failure.
// Any other kind of error will 500.
func handleCommand(ctx context.Context, w http.ResponseWriter, commandName string, args ...string) {
	cmd := exec.CommandContext(ctx, commandName, args...)
	output, err := cmd.CombinedOutput()

	var execError *exec.Error
	var exitErr *exec.ExitError

	if err != nil {
		// don't 500 for ExitErrors etc, to report "normal" failures to the flare log file
		if !errors.As(err, &execError) && !errors.As(err, &exitErr) {
			w.WriteHeader(500)
		}
		fmt.Fprintf(w, "command failed: %s\n%s", err, output)
		return
	}

	w.Write(output)
}

// HandleSelinuxSestatus reports the output of sestatus as an http result
func HandleSelinuxSestatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	handleCommand(ctx, w, "sestatus")
}

// HandleSelinuxSemoduleList reports the output of semodule -l as an http result
func HandleSelinuxSemoduleList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	handleCommand(ctx, w, "semodule", "-l")
}
