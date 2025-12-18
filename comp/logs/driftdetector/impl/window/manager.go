// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package window

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
)

// Manager handles sliding window aggregation of log entries
type Manager struct {
	config     common.WindowConfig
	inputChan  chan common.LogEntry
	outputChan chan common.Window
	ctx        context.Context
	cancel     context.CancelFunc

	currentWindow *common.Window
	windowID      int
}

// NewManager creates a new window manager
func NewManager(config common.WindowConfig, inputChan chan common.LogEntry, outputChan chan common.Window) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		config:     config,
		inputChan:  inputChan,
		outputChan: outputChan,
		ctx:        ctx,
		cancel:     cancel,
		windowID:   0,
	}
}

// Start begins processing log entries into windows
func (m *Manager) Start() {
	go m.run()
}

// Stop stops the window manager
func (m *Manager) Stop() {
	m.cancel()
}

func (m *Manager) run() {
	ticker := time.NewTicker(m.config.Step)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			// Flush any remaining window
			if m.currentWindow != nil && len(m.currentWindow.Logs) > 0 {
				m.outputChan <- *m.currentWindow
			}
			close(m.outputChan)
			return

		case entry := <-m.inputChan:
			m.addToWindow(entry)

		case <-ticker.C:
			// Time to slide the window
			if m.currentWindow != nil && len(m.currentWindow.Logs) > 0 {
				// Send the completed window
				windowCopy := *m.currentWindow
				m.outputChan <- windowCopy
			}

			// Create a new window
			now := time.Now()
			m.windowID++
			m.currentWindow = &common.Window{
				ID:        m.windowID,
				StartTime: now,
				EndTime:   now.Add(m.config.Size),
				Logs:      make([]common.LogEntry, 0, 1000),
			}
		}
	}
}

func (m *Manager) addToWindow(entry common.LogEntry) {
	// Initialize window if needed
	if m.currentWindow == nil {
		m.windowID++
		now := time.Now()
		m.currentWindow = &common.Window{
			ID:        m.windowID,
			StartTime: now,
			EndTime:   now.Add(m.config.Size),
			Logs:      make([]common.LogEntry, 0, 1000),
		}
	}

	// Add log to current window
	m.currentWindow.Logs = append(m.currentWindow.Logs, entry)
}
