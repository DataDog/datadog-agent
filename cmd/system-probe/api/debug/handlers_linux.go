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
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var klogRegexp = regexp.MustCompile(`<(\d+)>(.*)`)

var klogLevels = []string{
	"emerg",
	"alert",
	"crit",
	"err",
	"warn",
	"notice",
	"info",
	"debug",
}

// lowest 3 bits are the log level, remaining bits are the facility
const klogFacilityShift = 3
const klogLevelMask = (1 << klogFacilityShift) - 1

func klogLevelName(level int) string {
	return klogLevels[level&klogLevelMask]
}

func readAllDmesg() ([]byte, error) {
	n, err := syscall.Klogctl(unix.SYSLOG_ACTION_SIZE_BUFFER, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query size of log buffer [%w]", err)
	}

	b := make([]byte, n)

	m, err := syscall.Klogctl(unix.SYSLOG_ACTION_READ_ALL, b)
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

		levelName := "info"
		message := line

		// convert the numeric log level to a string
		parts := klogRegexp.FindStringSubmatch(line)
		if parts != nil {
			message = parts[2]

			digits := parts[1]
			level, err := strconv.Atoi(digits)
			if err == nil {
				levelName = klogLevelName(level)
			}
		}

		result += fmt.Sprintf("%-6s: %s\n", levelName, message)
	}

	return result, nil
}

// HandleLinuxDmesg writes linux dmesg into the HTTP response.
func HandleLinuxDmesg(w http.ResponseWriter, _ *http.Request) {
	dmesg, err := readAllDmesg()
	if err != nil {
		http.Error(w, "failed to read dmesg: "+err.Error(), http.StatusInternalServerError)
		return
	}

	dmesgStr, err := parseDmesg(dmesg)
	if err != nil {
		http.Error(w, "failed to parse dmesg: "+err.Error(), http.StatusInternalServerError)
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
		msg := "command failed: " + err.Error() + "\n" + string(output)
		if !errors.As(err, &execError) && !errors.As(err, &exitErr) {
			http.Error(w, msg, http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			io.WriteString(w, msg)
		}
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
