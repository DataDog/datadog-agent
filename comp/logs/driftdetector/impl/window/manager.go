// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package window

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// sourceWindowState tracks window state for a single source
type sourceWindowState struct {
	currentWindow *common.Window
	lastLogTime   time.Time
}

// Manager handles sliding window aggregation of log entries from multiple sources
type Manager struct {
	config     common.WindowConfig
	inputChan  chan common.LogEntry
	outputChan chan common.Window
	ctx        context.Context
	cancel     context.CancelFunc

	mu              sync.RWMutex
	sourceWindows   map[string]*sourceWindowState // Per-source window tracking
	windowIDCounter int
}

// NewManager creates a new window manager that handles multiple sources
func NewManager(config common.WindowConfig, inputChan chan common.LogEntry, outputChan chan common.Window) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		config:        config,
		inputChan:     inputChan,
		outputChan:    outputChan,
		ctx:           ctx,
		cancel:        cancel,
		sourceWindows: make(map[string]*sourceWindowState),
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

// ProcessLog sends a log to the shared window manager with source identification
func (m *Manager) ProcessLog(sourceKey string, timestamp time.Time, content string) {
	select {
	case m.inputChan <- common.LogEntry{
		SourceKey: sourceKey,
		Timestamp: timestamp,
		Content:   content,
	}:
	default:
		// Drop if channel full (back-pressure)
	}
}

func (m *Manager) run() {
	ticker := time.NewTicker(m.config.Step)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			// Flush any remaining windows
			m.flushAllWindows()
			close(m.outputChan)
			return

		case entry, ok := <-m.inputChan:
			if !ok {
				return
			}
			m.addLogToWindow(entry)

		case <-ticker.C:
			// Time to slide windows for all sources
			m.flushExpiredWindows()
		}
	}
}

func (m *Manager) addLogToWindow(entry common.LogEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sourceKey := entry.SourceKey
	state, exists := m.sourceWindows[sourceKey]

	if !exists || state.currentWindow == nil {
		// Create new window for this source
		m.windowIDCounter++
		state = &sourceWindowState{
			currentWindow: &common.Window{
				SourceKey: sourceKey,
				ID:        m.windowIDCounter,
				StartTime: entry.Timestamp,
				EndTime:   entry.Timestamp.Add(m.config.Size),
				Logs:      make([]common.LogEntry, 0, 1000),
			},
			lastLogTime: entry.Timestamp,
		}
		m.sourceWindows[sourceKey] = state
	}

	// Add log to current window for this source
	state.currentWindow.Logs = append(state.currentWindow.Logs, entry)
	state.lastLogTime = entry.Timestamp
}

func (m *Manager) flushExpiredWindows() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	for sourceKey, state := range m.sourceWindows {
		if state.currentWindow != nil {
			// Check if window duration has been reached
			if now.Sub(state.currentWindow.StartTime) >= m.config.Size {
				state.currentWindow.EndTime = now

				// Send window to shared template extractor (even if empty)
				// Empty windows are important for time-series continuity in DMD analysis
				select {
				case m.outputChan <- *state.currentWindow:
				default:
					log.Warnf("Dropping window for source %s (output channel full)", sourceKey)
				}

				// Create new window for this source
				m.windowIDCounter++
				state.currentWindow = &common.Window{
					SourceKey: sourceKey,
					ID:        m.windowIDCounter,
					StartTime: now,
					EndTime:   now.Add(m.config.Size),
					Logs:      make([]common.LogEntry, 0, 1000),
				}
			}
		}
	}
}

func (m *Manager) flushAllWindows() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Flush all windows on shutdown, including empty ones for time-series continuity
	for _, state := range m.sourceWindows {
		if state.currentWindow != nil {
			m.outputChan <- *state.currentWindow
		}
	}
}
