// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	doctordef "github.com/DataDog/datadog-agent/comp/doctor/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Init is called once when the program starts
// It returns an initial command to run (we start by fetching data)
func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,              // Start the spinner animation
		tick(),                      // Start the periodic ticker
		fetchDoctorStatus(m.client), // Fetch initial data immediately
	)
}

// Update handles incoming messages and updates the model state
// This is the Update function in the Model-Update-View architecture
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	if m.viewMode == LogsDetailView {
		cmds = append(cmds, readLogs(m.logChunk))
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Terminal was resized - update dimensions for responsive layout
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		// Handle keyboard input based on current view mode
		switch m.viewMode {
		case MainView:
			switch msg.String() {
			case "q", "ctrl+c":
				// User wants to quit
				m.quitting = true
				return m, tea.Quit

			case "r":
				// User requested manual refresh
				return m, fetchDoctorStatus(m.client)

			case "left", "h":
				// Navigate to previous panel
				if m.selectedPanel > 0 {
					m.selectedPanel--
				}

			case "right", "l":
				// Navigate to next panel
				if m.selectedPanel < 2 {
					m.selectedPanel++
				}

			case "enter":
				// Drill down into selected panel
				if m.selectedPanel == 0 && m.status != nil {
					// Enter logs detail view
					m.viewMode = LogsDetailView
					m.selectedLogIdx = 0
					// Start logs streaming goroutine

					// Start streaming logs for the first source if available
					if len(m.status.Ingestion.Logs.Integrations) > 0 {
						selectedSource := m.status.Ingestion.Logs.Integrations[m.selectedLogIdx]
						m.streamingSource = selectedSource.Name
						// Clear previous logs
						m.logLines = []string{}

						// creating new context
						ctx, cncl := context.WithCancel(context.Background())
						m.cmdCtx = ctx
						m.cmdCncl = cncl
						// TODO check erro
						logChan, err := startStreamLogs(ctx, m.client, selectedSource.Name)
						if err != nil {
							// TODO
						}
						m.logChunk = m.logChunk.Reset(logChan)
					}
				}
			}

		case LogsDetailView:
			switch msg.String() {
			case "q", "ctrl+c":
				// User wants to quit
				m.quitting = true
				return m, tea.Quit

			case "esc":
				// Go back to main view
				m.viewMode = MainView
				m.streamingSource = "" // Stop streaming
				// Clear previous logs
				m.logLines = []string{}
				if m.cmdCncl != nil {
					m.cmdCncl()
				}
				m.cmdCtx = nil
				m.cmdCncl = nil

			case "up", "k":
				// Navigate up in logs list
				if m.selectedLogIdx > 0 && m.status != nil {
					m.selectedLogIdx--
					// Switch to streaming logs for the newly selected source
					selectedSource := m.status.Ingestion.Logs.Integrations[m.selectedLogIdx]
					if m.streamingSource != selectedSource.Name {
						m.streamingSource = selectedSource.Name
						// Clear previous logs
						m.logLines = []string{}
						if m.cmdCncl != nil {
							m.cmdCncl()
						}

						// creating new context
						ctx, cncl := context.WithCancel(context.Background())
						m.cmdCtx = ctx
						m.cmdCncl = cncl
						// TODO check erro
						logChan, err := startStreamLogs(ctx, m.client, selectedSource.Name)
						if err != nil {
							// TODO
						}
						m.logChunk = m.logChunk.Reset(logChan)
					}
				}

			case "down", "j":
				// Navigate down in logs list
				if m.status != nil && m.selectedLogIdx < len(m.status.Ingestion.Logs.Integrations)-1 {
					m.selectedLogIdx++
					// Switch to streaming logs for the newly selected source
					selectedSource := m.status.Ingestion.Logs.Integrations[m.selectedLogIdx]
					if m.streamingSource != selectedSource.Name {
						m.streamingSource = selectedSource.Name
						// Clear previous logs
						m.logLines = []string{}
						if m.cmdCncl != nil {
							m.cmdCncl()
						}

						// creating new context
						ctx, cncl := context.WithCancel(context.Background())
						m.cmdCtx = ctx
						m.cmdCncl = cncl
						// TODO check erro
						logChan, err := startStreamLogs(ctx, m.client, selectedSource.Name)
						if err != nil {
							// TODO
						}
						m.logChunk = m.logChunk.Reset(logChan)
					}
				}
			}
		}

	case tickMsg:
		// Periodic timer fired - fetch new data
		return m, tea.Batch(
			tick(),                      // Schedule next tick
			fetchDoctorStatus(m.client), // Fetch new data
		)

	case fetchSuccessMsg:
		// Successfully fetched doctor status
		m.status = &msg.status
		m.lastError = nil
		m.loading = false
		m.lastUpdate = time.Now()

	case fetchErrorMsg:
		// Failed to fetch doctor status
		m.lastError = msg.err
		m.loading = false
		// Don't clear existing status - keep showing stale data with error indicator

	case refreshRequestMsg:
		// Manual refresh requested
		return m, fetchDoctorStatus(m.client)

	case logMsg:
		for m.logChunk.Scan() {
			m.logLines = append(m.logLines, m.logChunk.Text())
		}

		// Keep only the last maxLogLines
		if len(m.logLines) > m.maxLogLines {
			m.logLines = m.logLines[len(m.logLines)-m.maxLogLines:]
		}

	case streamErrorMsg:
		// Error streaming logs - just log it, don't fail
		// The user can still navigate the UI
		m.lastError = msg.err

	case spinner.TickMsg:
		// Update spinner animation
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// tick returns a command that sends a tickMsg after 2 seconds
// This drives the periodic refresh cycle
func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// fetchDoctorStatus returns a command that fetches doctor status from the agent
// This is executed asynchronously and sends a message when complete
func fetchDoctorStatus(client ipc.HTTPClient) tea.Cmd {
	return func() tea.Msg {
		// Create IPC endpoint for /agent/doctor
		endpoint, err := client.NewIPCEndpoint("/agent/doctor")
		if err != nil {
			return fetchErrorMsg{err: err}
		}

		// Fetch the data with a reasonable timeout
		res, err := endpoint.DoGet()
		if err != nil {
			return fetchErrorMsg{err: err}
		}

		// Parse the JSON response
		var status doctordef.DoctorStatus
		if err := json.Unmarshal(res, &status); err != nil {
			return fetchErrorMsg{err: err}
		}

		return fetchSuccessMsg{status: status}
	}
}

// streamLogs returns a command that streams logs from a specific source
// This connects to the /agent/stream-logs endpoint using PostChunk callback
// It runs in a goroutine and sends log lines back via tea.Cmd
func startStreamLogs(ctx context.Context, client ipc.HTTPClient, sourceName string) (chan []byte, error) {
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

	logChan := make(chan []byte)
	go func() {
		_ = client.PostChunk(url, "application/json", bytes.NewBuffer(filtersJSON),
			func(chunk []byte) {

				logChan <- chunk
			},
			httphelpers.WithContext(ctx))

		<-ctx.Done()
	}()

	return logChan, nil
}

func readLogs(logChunk *logChunk) tea.Cmd {
	return func() tea.Msg {
		if logChunk.ReadChan() {
			return logMsg{}
		}
		return nil
	}
}
