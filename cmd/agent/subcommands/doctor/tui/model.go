// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"bufio"
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"

	ipcdef "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	doctordef "github.com/DataDog/datadog-agent/comp/doctor/def"
)

// ViewMode represents the current view state
type ViewMode int

const (
	// MainView shows the three-panel dashboard
	MainView ViewMode = iota
	// LogsDetailView shows detailed logs information
	LogsDetailView
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

	// Navigation state
	viewMode       ViewMode // Current view mode
	selectedPanel  int      // Which panel is focused (0=ingestion, 1=agent, 2=intake)
	selectedLogIdx int      // Which log source is selected in detail view

	// Log streaming state
	logChunk    *logChunk // scanner for the log
	logLines    []string  // Buffered log lines for the selected source
	maxLogLines int       // Maximum number of log lines to keep

	streamingSource string          // Name of the currently streaming log source
	cmdCtx          context.Context // context used for long running command
	cmdCncl         func()          // context cancel function used for long running command
}

type logChunk struct {
	sync.Mutex
	logChunkChan chan []byte
	buf          bytes.Buffer
	scanner      *bufio.Scanner
}

func newLogChunk() *logChunk {
	buf := bytes.Buffer{}
	return &logChunk{
		logChunkChan: make(chan []byte),
		buf:          buf,
		scanner:      nil,
	}
}

func (lc *logChunk) ReadChan() bool {
	if lc == nil {
		return false
	}
	log, ok := <-lc.logChunkChan
	if !ok {
		return false
	}
	lc.Lock()
	lc.buf.Write(log)
	lc.Unlock()
	return true
}

func (lc *logChunk) Scan() bool {
	lc.Lock()
	defer lc.Unlock()
	return lc.scanner.Scan()
}

func (lc *logChunk) Text() string {
	lc.Lock()
	defer lc.Unlock()
	if lc.scanner == nil {
		return ""
	}
	return lc.scanner.Text()
}

func (lc *logChunk) Reset(logChunkChan chan []byte) *logChunk {
	if lc == nil {
		logChunk := *newLogChunk()
		logChunk.logChunkChan = logChunkChan
		return &logChunk
	}

	lc.Lock()
	defer lc.Unlock()
	close(lc.logChunkChan)
	lc.buf.Reset()
	lc.scanner = bufio.NewScanner(&lc.buf)
	lc.logChunkChan = logChunkChan
	return lc
}

// newModel creates a new model with initial state
func newModel(client ipcdef.HTTPClient) model {
	// Create a spinner for the loading state
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	return model{
		client:          client,
		status:          nil,
		lastError:       nil,
		width:           0,
		height:          0,
		loading:         true,
		spinner:         s,
		lastUpdate:      time.Time{},
		quitting:        false,
		viewMode:        MainView,
		selectedPanel:   0,
		selectedLogIdx:  0,
		logLines:        []string{},
		maxLogLines:     100, // Keep last 100 log lines
		streamingSource: "",
	}
}
