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
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
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
	// ServiceDetailView shows detailed view for a specific service with metrics and log tail
	ServiceDetailView
	// // LogsDetailView shows detailed logs information
	// LogsDetailView
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
	viewMode ViewMode // Current view mode
	// selectedPanel      int      // Which panel is focused (0=services, 1=agent)
	selectedLogIdx     int // Which log source is selected in detail view
	selectedServiceIdx int // Which service is selected in services view
	scrollOffset       int // Vertical scroll offset for services view

	// Log streaming state
	streamingSource string      // Name of the currently streaming log source
	logFetcher      *logFetcher // scanner for the log
	logLines        []string    // Buffered log lines for the selected source
	maxLogLines     int         // Maximum number of log lines to keep

	// Service detail view state
	selectedServiceForDetail string              // Service name being viewed in detail
	allLogsStreamFetcher     *logFetcher         // Single log fetcher for all logs
	serviceLogLinesBySource  map[string][]string // Map of service name -> log lines

	// Time-series data for dot graph visualization
	serviceTimeSeries map[string]*serviceTimeSeries // Service name -> time series data
	maxTimeSeriesLen  int                           // Maximum number of time buckets to track
	// otherTimeSeries   *serviceTimeSeries            // Aggregated data for unattributed activity

	// Animation state for endpoint connectivity visualization
	endpointPayloads       map[string][]*payloadAnimation // Endpoint URL -> list of active payloads
	endpointFlashState     map[string]*flashState         // Endpoint URL -> color flash state
	lastAnimationTrigger   map[string]time.Time           // Endpoint URL -> last animation time (for rate limiting)
	lastActivityTime       map[string]time.Time           // Endpoint URL -> last time endpoint had activity (for filtering)
	previousSuccessCounts  map[string]int64               // Endpoint URL -> previous success count (for detecting new sends)
	previousFailureCounts  map[string]int64               // Endpoint URL -> previous failure count (for detecting failures)
	previousRequeuedCounts map[string]int64               // Endpoint URL -> previous requeue count (for detecting requeues)
	previousErrorCounts    map[string]int64               // Endpoint URL -> previous error count (for detecting errors)

	// Flare state
	sendingFlare    bool            // True when flare is being sent
	flareResult     string          // Result message from flare command (empty, success, or error)
	flareEmailMode  bool            // True when prompting for email input
	flareEmailInput textinput.Model // Text input for email address
}

type logFetcher struct {
	sourceName  string // Name of the log source (for routing messages)
	filtersJSON []byte
	url         string
	client      ipc.HTTPClient

	msgChan chan tea.Msg       // Channel for sending messages to Bubbletea
	cancel  context.CancelFunc // Function to cancel the streaming
	done    chan struct{}      // Signals when stream is fully stopped
}

func newLogFetcher(sourceName string, client ipc.HTTPClient) (*logFetcher, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	// Create filters - if sourceName is empty, get all logs, otherwise filter by source name
	filters := map[string]string{}
	if sourceName != "" {
		filters["name"] = sourceName
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

	return &logFetcher{
		sourceName:  sourceName,
		client:      client,
		filtersJSON: filtersJSON,
		url:         url,
		msgChan:     make(chan tea.Msg, 10), // Buffered channel to prevent blocking
		done:        make(chan struct{}),
	}, nil
}

// StartStreaming starts the log streaming in a background goroutine and returns a command
// that continuously receives messages from the stream
func (lc *logFetcher) StartStreaming() tea.Cmd {
	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	lc.cancel = cancel

	// Start the HTTP streaming in a goroutine
	go func() {
		defer close(lc.done)
		defer close(lc.msgChan)

		log.Printf("Starting log stream for source %s from %s\n", lc.sourceName, lc.url)

		// Buffer for accumulating data
		var buf bytes.Buffer
		scanner := bufio.NewScanner(&buf)

		// Start streaming HTTP request
		err := lc.client.PostChunk(lc.url, "application/json", bytes.NewBuffer(lc.filtersJSON),
			func(chunk []byte) {
				// Process each chunk received
				buf.Write(chunk)
				scanner = bufio.NewScanner(&buf)

				// Map of service -> messages for this chunk
				serviceMessages := make(map[string][]string)

				for scanner.Scan() {
					line := scanner.Text()
					// Extract service and message from the diagnostic format
					// Format: "... | Service: <service> | ... | Message: <actual message>"
					serviceName, message := extractLogFields(line)
					if message != "" {
						serviceMessages[serviceName] = append(serviceMessages[serviceName], message)
					}
				}

				// Send one message per service that had logs in this chunk
				for service, messages := range serviceMessages {
					if len(messages) > 0 {
						log.Printf("Received %d log lines for service %s\n", len(messages), service)
						// Send message to Bubbletea (non-blocking due to buffered channel)
						select {
						case lc.msgChan <- logMsg{
							sourceName: service, // Now contains actual service name
							logLines:   messages,
						}:
						case <-ctx.Done():
							log.Printf("Stream cancelled\n")
							return
						}
					}
				}

				// Clear processed data from buffer
				buf.Reset()
				for scanner.Scan() {
					buf.WriteString(scanner.Text() + "\n")
				}
			},
			httphelpers.WithContext(ctx))

		if err != nil {
			log.Printf("Stream error for source %s: %v\n", lc.sourceName, err)
			select {
			case lc.msgChan <- streamErrorMsg{err: err}:
			case <-ctx.Done():
			}
		}
	}()

	// Return a command that waits for messages from the stream
	return lc.readNextMessage()
}

// extractLogFields extracts service name and message content from the diagnostic log format
// Format: "Integration Name: ... | Service: <service> | ... | Message: <actual message>\n"
// Returns (serviceName, message)
func extractLogFields(formattedLine string) (string, string) {
	// Extract service
	const servicePrefix = "Service: "
	serviceIdx := strings.Index(formattedLine, servicePrefix)
	var serviceName string
	if serviceIdx != -1 {
		// Find the end of the service field (next pipe or end of string)
		serviceStart := serviceIdx + len(servicePrefix)
		serviceEnd := strings.Index(formattedLine[serviceStart:], " |")
		if serviceEnd != -1 {
			serviceName = strings.TrimSpace(formattedLine[serviceStart : serviceStart+serviceEnd])
		} else {
			serviceName = strings.TrimSpace(formattedLine[serviceStart:])
		}
	}

	// Extract message
	const messagePrefix = "Message: "
	messageIdx := strings.Index(formattedLine, messagePrefix)
	var message string
	if messageIdx != -1 {
		// Extract everything after "Message: "
		message = strings.TrimSpace(formattedLine[messageIdx+len(messagePrefix):])
	} else {
		// If format doesn't match, return the whole line as message
		message = formattedLine
	}

	return serviceName, message
}

// readNextMessage returns a command that waits for the next message from the stream
func (lc *logFetcher) readNextMessage() tea.Cmd {
	return func() tea.Msg {
		// Block waiting for next message from the stream goroutine
		msg, ok := <-lc.msgChan
		if !ok {
			// Channel closed, stream ended
			log.Printf("Stream ended for source %s\n", lc.sourceName)
			return nil
		}
		return msg
	}
}

func (lc *logFetcher) Close() {
	if lc == nil {
		return
	}

	// Cancel the context to stop the HTTP stream
	if lc.cancel != nil {
		lc.cancel()
	}

	// Wait for the stream goroutine to finish
	<-lc.done
}

// payloadAnimation represents a single payload moving along the wire
type payloadAnimation struct {
	progress   float64   // Progress along wire: 0.0 (left) to 1.0 (right/reached)
	startTime  time.Time // When animation started
	arrowType  string    // "normal" (white) or "retry" (yellow) - determines arrow color
	resultType string    // "success" (white) or "failure" (red) - determines URL flash color
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

	// Create email input for flare
	emailInput := textinput.New()
	emailInput.Placeholder = "your.email@example.com"
	emailInput.Focus()
	emailInput.CharLimit = 100
	emailInput.Width = 50

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
		viewMode:   MainView,
		// selectedPanel:      0,
		selectedLogIdx:           0,
		selectedServiceIdx:       0,
		scrollOffset:             0,
		logLines:                 []string{},
		maxLogLines:              100, // Keep last 100 log lines
		streamingSource:          "",
		selectedServiceForDetail: "",
		allLogsStreamFetcher:     nil,
		serviceLogLinesBySource:  make(map[string][]string),
		serviceTimeSeries:        make(map[string]*serviceTimeSeries),
		maxTimeSeriesLen:         60, // Track last 60 time buckets (2 seconds per refresh = 2 minutes)
		// otherTimeSeries:    newServiceTimeSeries(60), // "other" service for unattributed activity
		endpointPayloads:       make(map[string][]*payloadAnimation),
		endpointFlashState:     make(map[string]*flashState),
		lastAnimationTrigger:   make(map[string]time.Time),
		lastActivityTime:       make(map[string]time.Time),
		previousSuccessCounts:  make(map[string]int64),
		previousFailureCounts:  make(map[string]int64),
		previousRequeuedCounts: make(map[string]int64),
		previousErrorCounts:    make(map[string]int64),
		sendingFlare:           false,
		flareResult:            "",
		flareEmailMode:         false,
		flareEmailInput:        emailInput,
	}
}
