// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package debug contains handlers for debug information global to all of system-probe
package debug

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"time"
)

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
