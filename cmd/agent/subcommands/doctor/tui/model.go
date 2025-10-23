// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcdef "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	doctordef "github.com/DataDog/datadog-agent/comp/doctor/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// ViewMode represents the current view state
type ViewMode int

const (
	// MainView shows the three-panel dashboard
	MainView ViewMode = iota
	// ServicesView shows the services list with their stats
	ServicesView
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
	viewMode           ViewMode // Current view mode
	selectedPanel      int      // Which panel is focused (0=services, 1=agent)
	selectedLogIdx     int      // Which log source is selected in detail view
	selectedServiceIdx int      // Which service is selected in services view
	scrollOffset       int      // Vertical scroll offset for services view

	// Log streaming state
	streamingSource string      // Name of the currently streaming log source
	logFetcher      *logFetcher // scanner for the log
	logLines        []string    // Buffered log lines for the selected source
	maxLogLines     int         // Maximum number of log lines to keep

	// Time-series data for dot graph visualization
	serviceTimeSeries map[string]*serviceTimeSeries // Service name -> time series data
	maxTimeSeriesLen  int                           // Maximum number of time buckets to track
	// otherTimeSeries   *serviceTimeSeries            // Aggregated data for unattributed activity

	// Animation state for endpoint connectivity visualization
	endpointPayloads      map[string][]*payloadAnimation // Endpoint URL -> list of active payloads
	endpointFlashState    map[string]*flashState         // Endpoint URL -> color flash state
	lastAnimationTrigger  map[string]time.Time           // Endpoint URL -> last animation time (for rate limiting)
	lastActivityTime      map[string]time.Time           // Endpoint URL -> last time endpoint had activity (for filtering)
	previousSuccessCounts map[string]int64               // Endpoint URL -> previous success count (for detecting new sends)
	previousFailureCounts map[string]int64               // Endpoint URL -> previous failure count (for detecting failures)
}

type logFetcher struct {
	sync.Mutex

	// transitive data
	filtersJSON []byte
	url         string
	client      ipc.HTTPClient

	logChunkChan chan []byte
	cmdCtx       context.Context // context used for long running command
	cmdCncl      func()          // context cancel function used for long running command

	buf     bytes.Buffer
	scanner *bufio.Scanner
	wg      sync.WaitGroup
}

func newLogFetcher(sourceName string, client ipc.HTTPClient) (*logFetcher, error) {
	buf := bytes.Buffer{}

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	// Create filters for the specific source
	filters := map[string]string{
		"name": sourceName,
	}
	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return nil, err
	}

	// Build the URL for the stream-logs endpoint
	cmdPort := pkgconfigsetup.Datadog().GetInt("cmd_port")
	if cmdPort == 0 {
		cmdPort = 5001
	}
	url := fmt.Sprintf("https://%v:%v/agent/stream-logs", ipcAddress, cmdPort)

	// creating new context
	ctx, cncl := context.WithCancel(context.Background())
	return &logFetcher{
		client:       client,
		filtersJSON:  filtersJSON,
		url:          url,
		logChunkChan: make(chan []byte),
		cmdCtx:       ctx,
		cmdCncl:      cncl,
		buf:          buf,
		scanner:      bufio.NewScanner(&buf),
	}, nil
}

func (lc *logFetcher) ListenCmd() tea.Cmd {
	return func() tea.Msg {
		lc.wg.Add(1)
		defer lc.wg.Done()
		log.Printf("starting to stream log from %v\n", lc.url)
		lc.client.PostChunk(lc.url, "application/json", bytes.NewBuffer(lc.filtersJSON),
			func(chunk []byte) {
				lc.logChunkChan <- chunk
				log.Printf("Recieved chunk from %v\n", lc.url)
			},
			httphelpers.WithContext(lc.cmdCtx))
		return nil
	}
}

func (lc *logFetcher) WaitCmd() tea.Cmd {
	return func() tea.Msg {
		lc.wg.Add(1)
		defer lc.wg.Done()
		logChunk, ok := <-lc.logChunkChan
		if !ok {
			return nil
		}
		lc.Lock()
		defer lc.Unlock()
		log.Printf("Adding chunk to buffer %v\n", lc.url)

		lc.buf.Write(logChunk)
		res := []string{}
		for lc.scanner.Scan() {
			res = append(res, lc.scanner.Text())
		}
		if len(res) > 0 {
			log.Printf("Returning logMsg %v\n", lc.url)
			return logMsg{
				logLines: res,
			}
		}
		return nil
	}
}

func (lc *logFetcher) Close() {
	if lc == nil {
		return
	}

	// First stop context
	lc.cmdCncl()
	// wait before closing the channel
	lc.wg.Wait()

	// Then close channel
	close(lc.logChunkChan)
}

// payloadAnimation represents a single payload moving along the wire
type payloadAnimation struct {
	progress  float64   // Progress along wire: 0.0 (left) to 1.0 (right/reached)
	startTime time.Time // When animation started
	success   bool      // Will this delivery succeed or fail
}

// flashState represents a temporary color flash on an endpoint
type flashState struct {
	color     lipgloss.Color // White for success, red for error
	startTime time.Time      // When flash started
	duration  time.Duration  // How long to show the flash
}

// newModel creates a new model with initial state
func newModel(client ipcdef.HTTPClient) model {
	// Create a spinner for the loading state
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	return model{
		client:             client,
		status:             nil,
		lastError:          nil,
		width:              0,
		height:             0,
		loading:            true,
		spinner:            s,
		lastUpdate:         time.Time{},
		quitting:           false,
		viewMode:           MainView,
		selectedPanel:      0,
		selectedLogIdx:     0,
		selectedServiceIdx: 0,
		scrollOffset:       0,
		logLines:           []string{},
		maxLogLines:        100, // Keep last 100 log lines
		streamingSource:    "",
		serviceTimeSeries:  make(map[string]*serviceTimeSeries),
		maxTimeSeriesLen:   60, // Track last 60 time buckets (2 seconds per refresh = 2 minutes)
		// otherTimeSeries:    newServiceTimeSeries(60), // "other" service for unattributed activity
		endpointPayloads:      make(map[string][]*payloadAnimation),
		endpointFlashState:    make(map[string]*flashState),
		lastAnimationTrigger:  make(map[string]time.Time),
		lastActivityTime:      make(map[string]time.Time),
		previousSuccessCounts: make(map[string]int64),
		previousFailureCounts: make(map[string]int64),
	}
}
