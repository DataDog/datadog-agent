// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"time"

	ipcdef "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	doctordef "github.com/DataDog/datadog-agent/comp/doctor/def"
	"github.com/charmbracelet/bubbles/spinner"
)

// model represents the entire application state for the Bubbletea TUI
// This is the Model in the Model-Update-View architecture
type model struct {
	// IPC client for fetching doctor status from the agent
	client ipcdef.HTTPClient

	// Current doctor status data (nil until first successful fetch)
	status *doctordef.DoctorStatus

	// Last error encountered during fetch (nil if no error)
	lastError error

	// Terminal dimensions for responsive layout
	width  int
	height int

	// Loading state
	loading bool
	spinner spinner.Model

	// Timestamp of last successful update
	lastUpdate time.Time

	// Flag to indicate the TUI should quit
	quitting bool
}

// newModel creates a new model with initial state
func newModel(client ipcdef.HTTPClient) model {
	// Create a spinner for the loading state
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	return model{
		client:     client,
		status:     nil,
		lastError:  nil,
		width:      0,
		height:     0,
		loading:    true,
		spinner:    s,
		lastUpdate: time.Time{},
		quitting:   false,
	}
}
