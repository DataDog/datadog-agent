// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"encoding/json"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	doctordef "github.com/DataDog/datadog-agent/comp/doctor/def"
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

	// if m.viewMode == LogsDetailView {
	// 	cmds = append(cmds, readLogs(m.logFetcher))
	// }

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
				// Navigate to previous panel (only 2 panels now: services and agent)
				if m.selectedPanel > 0 {
					m.selectedPanel--
				}

			case "right", "l":
				// Navigate to next panel (only 2 panels now: services and agent)
				if m.selectedPanel < 1 {
					m.selectedPanel++
				}

			case "up", "k":
				// Navigate up in services list (only when services panel is selected)
				if m.selectedPanel == 0 && m.status != nil {
					if m.selectedServiceIdx > 0 {
						m.selectedServiceIdx--
						// Adjust scroll offset if needed
						if m.selectedServiceIdx < m.scrollOffset {
							m.scrollOffset = m.selectedServiceIdx
						}
					}
				}

			case "down", "j":
				// Navigate down in services list (only when services panel is selected)
				if m.selectedPanel == 0 && m.status != nil {
					maxIdx := len(m.status.Services) - 1
					if m.selectedServiceIdx < maxIdx {
						m.selectedServiceIdx++
						// Adjust scroll offset if needed
						linesPerService := 8
						panelHeight := m.height - 6
						availableHeight := panelHeight - 5
						maxVisibleServices := availableHeight / linesPerService
						if maxVisibleServices < 1 {
							maxVisibleServices = 1
						}
						if m.selectedServiceIdx >= m.scrollOffset+maxVisibleServices {
							m.scrollOffset = m.selectedServiceIdx - maxVisibleServices + 1
						}
					}
				}

			case "pgup":
				// Page up in services list (only when services panel is selected)
				if m.selectedPanel == 0 {
					m.selectedServiceIdx -= 10
					if m.selectedServiceIdx < 0 {
						m.selectedServiceIdx = 0
					}
					m.scrollOffset = m.selectedServiceIdx
				}

			case "pgdown":
				// Page down in services list (only when services panel is selected)
				if m.selectedPanel == 0 && m.status != nil {
					m.selectedServiceIdx += 10
					maxIdx := len(m.status.Services) - 1
					if m.selectedServiceIdx > maxIdx {
						m.selectedServiceIdx = maxIdx
					}
					linesPerService := 8
					panelHeight := m.height - 6
					availableHeight := panelHeight - 5
					maxVisibleServices := availableHeight / linesPerService
					if maxVisibleServices < 1 {
						maxVisibleServices = 1
					}
					if m.selectedServiceIdx >= m.scrollOffset+maxVisibleServices {
						m.scrollOffset = m.selectedServiceIdx - maxVisibleServices + 1
					}
				}

			case "home":
				// Jump to first service (only when services panel is selected)
				if m.selectedPanel == 0 {
					m.selectedServiceIdx = 0
					m.scrollOffset = 0
				}

			case "end":
				// Jump to last service including "other" (only when services panel is selected)
				if m.selectedPanel == 0 && m.status != nil {
					m.selectedServiceIdx = len(m.status.Services) - 1 // Points to "other"
					linesPerService := 8
					panelHeight := m.height - 6
					availableHeight := panelHeight - 5
					maxVisibleServices := availableHeight / linesPerService
					if maxVisibleServices < 1 {
						maxVisibleServices = 1
					}
					m.scrollOffset = m.selectedServiceIdx - maxVisibleServices + 1
					if m.scrollOffset < 0 {
						m.scrollOffset = 0
					}
				}
			}

		case ServicesView:
			switch msg.String() {
			case "q", "ctrl+c":
				// User wants to quit
				m.quitting = true
				return m, tea.Quit

			case "esc":
				// Go back to main view
				m.viewMode = MainView

			case "r":
				// User requested manual refresh
				return m, fetchDoctorStatus(m.client)

			case "up", "k":
				// Navigate up in services list
				if m.selectedServiceIdx > 0 {
					m.selectedServiceIdx--
					// Adjust scroll offset if needed
					if m.selectedServiceIdx < m.scrollOffset {
						m.scrollOffset = m.selectedServiceIdx
					}
				}

			case "down", "j":
				// Navigate down in services list
				if m.status != nil && m.selectedServiceIdx < len(m.status.Services)-1 {
					m.selectedServiceIdx++
					// Adjust scroll offset if needed
					linesPerService := 15
					availableHeight := m.height - 6
					maxVisibleServices := availableHeight / linesPerService
					if m.selectedServiceIdx >= m.scrollOffset+maxVisibleServices {
						m.scrollOffset = m.selectedServiceIdx - maxVisibleServices + 1
					}
				}

			case "pgup":
				// Page up (10 services at a time)
				m.selectedServiceIdx -= 10
				if m.selectedServiceIdx < 0 {
					m.selectedServiceIdx = 0
				}
				m.scrollOffset = m.selectedServiceIdx

			case "pgdown":
				// Page down (10 services at a time)
				if m.status != nil {
					m.selectedServiceIdx += 10
					maxIdx := len(m.status.Services) - 1
					if m.selectedServiceIdx > maxIdx {
						m.selectedServiceIdx = maxIdx
					}
					linesPerService := 15
					availableHeight := m.height - 6
					maxVisibleServices := availableHeight / linesPerService
					if m.selectedServiceIdx >= m.scrollOffset+maxVisibleServices {
						m.scrollOffset = m.selectedServiceIdx - maxVisibleServices + 1
					}
				}

			case "home":
				// Jump to first service
				m.selectedServiceIdx = 0
				m.scrollOffset = 0

			case "end":
				// Jump to last service (including "other")
				if m.status != nil {
					m.selectedServiceIdx = len(m.status.Services) - 1 // Points to "other"
					linesPerService := 15
					availableHeight := m.height - 6
					maxVisibleServices := availableHeight / linesPerService
					m.scrollOffset = m.selectedServiceIdx - maxVisibleServices + 1
					if m.scrollOffset < 0 {
						m.scrollOffset = 0
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
						m.logFetcher.Close()
						logFetcher, err := newLogFetcher(selectedSource.Name, m.client)
						if err != nil {
							// TODO
						}
						m.logFetcher = logFetcher
						return m, tea.Batch(m.logFetcher.ListenCmd(), m.logFetcher.WaitCmd())
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
						m.logFetcher.Close()
						logFetcher, err := newLogFetcher(selectedSource.Name, m.client)
						if err != nil {
							// TODO
						}
						m.logFetcher = logFetcher
						return m, tea.Batch(m.logFetcher.ListenCmd(), m.logFetcher.WaitCmd())
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

		// Update time series data for all services
		m.updateTimeSeriesData()

	case fetchErrorMsg:
		// Failed to fetch doctor status
		m.lastError = msg.err
		m.loading = false
		// Don't clear existing status - keep showing stale data with error indicator

	case refreshRequestMsg:
		// Manual refresh requested
		return m, fetchDoctorStatus(m.client)

	case logMsg:
		m.logLines = append(m.logLines, msg.logLines...)

		// Keep only the last maxLogLines
		if len(m.logLines) > m.maxLogLines {
			m.logLines = m.logLines[len(m.logLines)-m.maxLogLines:]
		}
		return m, m.logFetcher.WaitCmd()

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
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
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

// updateTimeSeriesData updates the rolling time series buffers with latest service data
func (m *model) updateTimeSeriesData() {
	if m.status == nil {
		return
	}

	// Track which services we've seen in this update
	seenServices := make(map[string]bool)

	// // Aggregate unattributed activity (services with empty names)
	// var otherMetrics, otherLogs, otherTraces float64

	// Update time series for each service
	for _, service := range m.status.Services {
		// if service.Name == "" {
		// 	// Accumulate unattributed activity
		// 	otherMetrics += service.MetricsRate
		// 	otherLogs += service.LogsRate
		// 	otherTraces += service.TracesRate
		// 	continue
		// }

		seenServices[service.Name] = true

		// Get or create time series for this service
		ts, exists := m.serviceTimeSeries[service.Name]
		if !exists {
			ts = newServiceTimeSeries(m.maxTimeSeriesLen)
			m.serviceTimeSeries[service.Name] = ts
		}

		// Add latest values to the time series
		ts.metrics.add(service.MetricsRate)
		ts.logs.add(service.LogsRate)
		ts.traces.add(service.TracesRate)
	}

	// // Update "other" time series
	// m.otherTimeSeries.metrics.add(otherMetrics)
	// m.otherTimeSeries.logs.add(otherLogs)
	// m.otherTimeSeries.traces.add(otherTraces)

	// Add zero values for services that disappeared (to maintain time continuity)
	for serviceName, ts := range m.serviceTimeSeries {
		if !seenServices[serviceName] {
			ts.metrics.add(0)
			ts.logs.add(0)
			ts.traces.add(0)
		}
	}
}
