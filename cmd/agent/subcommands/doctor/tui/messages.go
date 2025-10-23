// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"time"

	doctordef "github.com/DataDog/datadog-agent/comp/doctor/def"
)

// Message types for the Bubbletea update loop
// Following the Elm Architecture pattern, all state changes flow through these messages

// tickMsg is sent by the ticker to trigger periodic refresh
// This drives the 2-second polling cycle
type tickMsg time.Time

// fetchSuccessMsg is sent when doctor status is successfully fetched
// Contains the parsed DoctorStatus data to be rendered
type fetchSuccessMsg struct {
	status doctordef.DoctorStatus
}

// fetchErrorMsg is sent when fetching doctor status fails
// Contains the error to display to the user
type fetchErrorMsg struct {
	err error
}

// refreshRequestMsg is sent when the user manually requests a refresh (via 'r' key)
// This bypasses the normal ticker cycle for immediate feedback
type refreshRequestMsg struct{}

// logMsg is sent when a new chunk of the log is received from the stream
type logMsg struct {
	logLines []string
}

// streamErrorMsg is sent when there's an error streaming logs
type streamErrorMsg struct {
	err error
}

// animationRefreshMsg is sent when the animation refresh loop should run
type animationRefreshMsg struct{}
