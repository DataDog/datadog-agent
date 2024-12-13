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

// HandleSelinuxSestatus reports the output of sestatus as an http result
func HandleSelinuxSestatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sestatus")
	output, err := cmd.CombinedOutput()

	var execError *exec.Error
	var exitErr *exec.ExitError

	if err != nil {
		// don't 500 for ExitErrors etc, to report "normal" failures to the selinux_sestatus.log file
		if !errors.As(err, &execError) && !errors.As(err, &exitErr) {
			w.WriteHeader(500)
		}
		fmt.Fprintf(w, "command failed: %s\n%s", err, output)
		return
	}

	w.Write(output)
}
