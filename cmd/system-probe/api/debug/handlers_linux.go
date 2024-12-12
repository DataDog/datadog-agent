// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package debug contains handlers for debug information global to all of system-probe
package debug

import (
	"errors"
	"fmt"
	"net/http"
	"os/exec"
)

// HandleSelinuxSestatus reports the output of sestatus as an http result
func HandleSelinuxSestatus(w http.ResponseWriter, _ *http.Request) {
	cmd := exec.Command("sestatus")
	output, err := cmd.CombinedOutput()
	// don't report ExitErrors since we are using the combined output which will already include stderr
	if err != nil && !errors.Is(err, &exec.ExitError{}) {
		fmt.Fprintf(w, "sestatus command failed: %s", err)
		return
	}

	w.Write(output)
}
