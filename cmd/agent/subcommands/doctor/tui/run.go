// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tui implements an interactive terminal UI for the Datadog Agent doctor command
package tui

import (
	"fmt"

	ipcdef "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the interactive TUI for the doctor command
// This is the main entry point called from the Cobra command
func Run(client ipcdef.HTTPClient) error {
	// Create the initial model
	m := newModel(client)

	// Create the Bubbletea program with alternate screen buffer
	// Alternate screen buffer means the TUI doesn't interfere with terminal history
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support (optional enhancement)
	)

	// Run the program
	// This blocks until the user quits or an error occurs
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	// Check if there was an error in the final model state
	// This handles the case where the agent was never reachable
	if m, ok := finalModel.(model); ok {
		if m.lastError != nil && m.status == nil {
			return fmt.Errorf("could not connect to agent: %w", m.lastError)
		}
	}

	return nil
}
