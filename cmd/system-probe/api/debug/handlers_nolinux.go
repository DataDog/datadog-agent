// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux

// Package debug contains handlers for debug information global to all of system-probe
package debug

import (
	"io"
	"net/http"
)

// HandleSelinuxSestatus is not supported
func HandleSelinuxSestatus(w http.ResponseWriter, _ *http.Request) {
	io.WriteString(w, "HandleSelinuxSestatus is not supported on this platform")
	w.WriteHeader(500)
	return
}
