// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// View renders the current model state to a string for display
// This is the View function in the Model-Update-View architecture
func (m model) View() string {
	if m.quitting {
		return ""
	}

	// Handle error state - show full-screen error message
	if m.lastError != nil && m.status == nil {
		return m.renderError()
	}

	// Handle loading state - show spinner
	if m.loading && m.status == nil {
		return m.renderLoading()
	}

	// Render based on view mode
	switch m.viewMode {
	case ServicesView:
		return m.renderServicesView()
	case LogsDetailView:
		return m.renderLogsDetailView()
	default:
		return m.renderMainView()
	}
}

// renderError displays a full-screen error message when agent is not reachable
func (m model) renderError() string {
	errorMsg := fmt.Sprintf("Failed to connect to Datadog Agent\n\n%s\n\nMake sure the agent is running and try again.", m.lastError.Error())
	styledError := errorMessageStyle.Render(errorMsg)

	// Add footer with keyboard shortcuts
	footer := m.renderFooter()

	return lipgloss.JoinVertical(
		lipgloss.Center,
		"\n\n",
		styledError,
		"\n",
		footer,
	)
}

// renderLoading displays a loading spinner while fetching initial data
func (m model) renderLoading() string {
	loadingMsg := fmt.Sprintf("%s Connecting to Datadog Agent...", m.spinner.View())
	styledLoading := spinnerStyle.Render(loadingMsg)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		styledLoading,
	)
}

// renderMainView renders the four-panel layout with all doctor status information
func (m model) renderMainView() string {
	if m.status == nil {
		return "No data available"
	}

	// Calculate panel dimensions
	// Leave space for borders, padding, and margins
	// Formula: (width - margins - borders) / 4
	panelWidth := (m.width - 16) / 4 // 16 = 4 panels * (2 margin + 2 border)
	panelHeight := m.height - 6      // Leave space for header and footer

	// Render each panel with selection state
	servicesPanel := m.renderServicesPanel(panelWidth, panelHeight, m.selectedPanel == 0)
	ingestionPanel := m.renderIngestionPanel(panelWidth, panelHeight, m.selectedPanel == 1)
	agentPanel := m.renderAgentPanel(panelWidth, panelHeight, m.selectedPanel == 2)
	intakePanel := m.renderIntakePanel(panelWidth, panelHeight, m.selectedPanel == 3)

	// Join panels horizontally
	panels := lipgloss.JoinHorizontal(
		lipgloss.Top,
		servicesPanel,
		ingestionPanel,
		agentPanel,
		intakePanel,
	)

	// Add header and footer
	header := m.renderHeader()
	footer := m.renderFooter()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		panels,
		footer,
	)
}

// renderHeader displays the title and last update time
func (m model) renderHeader() string {
	title := titleStyle.Render("Datadog Agent Doctor")

	var status string
	if m.lastError != nil {
		status = errorStyle.Render(" [Connection Error]")
	} else if time.Since(m.lastUpdate) > 5*time.Second {
		status = warningStyle.Render(" [Stale Data]")
	} else {
		status = successStyle.Render(" [Connected]")
	}

	lastUpdate := ""
	if !m.lastUpdate.IsZero() {
		lastUpdate = subduedStyle.Render(fmt.Sprintf("Last updated: %s", m.lastUpdate.Format("15:04:05")))
	}

	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		status,
		"  ",
		lastUpdate,
	)

	return baseStyle.Render(header)
}

// renderFooter displays keyboard shortcuts based on current view mode
func (m model) renderFooter() string {
	var shortcuts []string

	switch m.viewMode {
	case LogsDetailView:
		shortcuts = []string{
			keyStyle.Render("↑/↓") + " navigate",
			keyStyle.Render("esc") + " back",
			keyStyle.Render("q") + " quit",
		}
	default: // MainView
		shortcuts = []string{
			keyStyle.Render("←/→") + " switch panel",
			keyStyle.Render("enter") + " details",
			keyStyle.Render("r") + " refresh",
			keyStyle.Render("q") + " quit",
		}
	}

	return footerStyle.Render(strings.Join(shortcuts, " • "))
}

// renderIngestionPanel renders the left panel showing ingestion status
func (m model) renderIngestionPanel(width, height int, isSelected bool) string {
	var content strings.Builder

	// Panel title with selection indicator
	title := "Ingestion"
	if isSelected {
		title = "▶ " + title
	}
	content.WriteString(titleStyle.Render(title) + "\n\n")

	// Checks section
	content.WriteString(formatSectionHeader("Checks") + "\n")
	checks := m.status.Ingestion.Checks
	content.WriteString(fmt.Sprintf("  %s\n", formatCount("Total", checks.Total, "")))
	content.WriteString(fmt.Sprintf("  %s\n", formatCount("Running", checks.Running, "success")))
	if checks.Errors > 0 {
		content.WriteString(fmt.Sprintf("  %s\n", formatCount("Errors", checks.Errors, "error")))
	}
	if checks.Warnings > 0 {
		content.WriteString(fmt.Sprintf("  %s\n", formatCount("Warnings", checks.Warnings, "warning")))
	}

	// Show individual check status (limit to 5 most recent)
	if len(checks.CheckList) > 0 {
		content.WriteString("\n  " + subduedStyle.Render("Recent checks:") + "\n")
		limit := 5
		if len(checks.CheckList) < limit {
			limit = len(checks.CheckList)
		}
		for i := 0; i < limit; i++ {
			check := checks.CheckList[i]
			status := formatStatusIndicator(check.Status, 0)
			content.WriteString(fmt.Sprintf("  %s %s\n", status, valueStyle.Render(check.Name)))
			if check.LastError != "" && check.Status == "error" {
				content.WriteString(fmt.Sprintf("    %s\n", errorStyle.Render(truncate(check.LastError, 30))))
			}
		}
	}

	// DogStatsD section
	content.WriteString("\n" + formatSectionHeader("DogStatsD") + "\n")
	dogstatsd := m.status.Ingestion.DogStatsD
	content.WriteString(fmt.Sprintf("  %s\n", formatKeyValue("Metrics", formatLargeNumber(dogstatsd.MetricsReceived))))
	content.WriteString(fmt.Sprintf("  %s\n", formatKeyValue("Packets", formatLargeNumber(dogstatsd.PacketsReceived))))
	if dogstatsd.PacketsDropped > 0 {
		content.WriteString(fmt.Sprintf("  %s\n", formatKeyValueStatus("Dropped", formatLargeNumber(dogstatsd.PacketsDropped), "warning")))
	}
	if dogstatsd.ParseErrors > 0 {
		content.WriteString(fmt.Sprintf("  %s\n", formatKeyValueStatus("Errors", formatLargeNumber(dogstatsd.ParseErrors), "error")))
	}

	// Logs section with detail hint
	logsHeader := "Logs"
	if isSelected {
		logsHeader += " " + subduedStyle.Render("(press Enter for details)")
	}
	content.WriteString("\n" + formatSectionHeader(logsHeader) + "\n")
	logs := m.status.Ingestion.Logs

	// Show enabled/disabled status
	if logs.Enabled {
		content.WriteString(fmt.Sprintf("  %s\n", formatKeyValueStatus("Status", "Enabled", "success")))
		content.WriteString(fmt.Sprintf("  %s\n", formatCount("Sources", logs.Sources, "")))
		content.WriteString(fmt.Sprintf("  %s\n", formatKeyValue("Lines", formatLargeNumber(logs.LinesProcessed))))
		content.WriteString(fmt.Sprintf("  %s\n", formatKeyValue("Bytes", formatBytes(logs.BytesProcessed))))
		if logs.Errors > 0 {
			content.WriteString(fmt.Sprintf("  %s\n", formatKeyValueStatus("Errors", fmt.Sprintf("%d", logs.Errors), "error")))
		}
	} else {
		content.WriteString(fmt.Sprintf("  %s\n", formatKeyValueStatus("Status", "Disabled", "warning")))
	}

	// Metrics section
	content.WriteString("\n" + formatSectionHeader("Metrics") + "\n")
	metrics := m.status.Ingestion.Metrics
	content.WriteString(fmt.Sprintf("  %s\n", formatKeyValue("In Queue", fmt.Sprintf("%d", metrics.InQueue))))
	content.WriteString(fmt.Sprintf("  %s\n", formatKeyValue("Flushed", formatLargeNumber(metrics.Flushed))))

	return panelStyle.
		Width(width).
		Height(height).
		Render(content.String())
}

// renderAgentPanel renders the center panel showing agent health and metadata
func (m model) renderAgentPanel(width, height int, isSelected bool) string {
	var content strings.Builder

	// Panel title with selection indicator
	title := "Agent Health"
	if isSelected {
		title = "▶ " + title
	}
	content.WriteString(titleStyle.Render(title) + "\n\n")

	// Running status
	agent := m.status.Agent
	if agent.Running {
		content.WriteString(fmt.Sprintf("%s\n\n", formatStatusIndicator("running", 0)))
	} else {
		content.WriteString(fmt.Sprintf("%s\n\n", formatStatusIndicator("error", 0)))
	}

	// Agent metadata
	content.WriteString(formatSectionHeader("Metadata") + "\n")
	content.WriteString(fmt.Sprintf("  %s\n", formatKeyValue("Version", agent.Version)))
	content.WriteString(fmt.Sprintf("  %s\n", formatKeyValue("Hostname", agent.Hostname)))
	content.WriteString(fmt.Sprintf("  %s\n", formatKeyValue("Uptime", formatDuration(agent.Uptime))))

	// Health status
	content.WriteString("\n" + formatSectionHeader("Component Health") + "\n")
	if len(agent.Health.Healthy) > 0 {
		content.WriteString("  " + successStyle.Render(symbolSuccess+" Healthy:") + "\n")
		for _, component := range agent.Health.Healthy {
			content.WriteString(fmt.Sprintf("    %s\n", valueStyle.Render(component)))
		}
	}
	if len(agent.Health.Unhealthy) > 0 {
		content.WriteString("  " + errorStyle.Render(symbolError+" Unhealthy:") + "\n")
		for _, component := range agent.Health.Unhealthy {
			content.WriteString(fmt.Sprintf("    %s\n", errorStyle.Render(component)))
		}
	}

	// Error count
	if agent.ErrorsLast5Min > 0 {
		content.WriteString("\n" + formatSectionHeader("Recent Errors") + "\n")
		content.WriteString(fmt.Sprintf("  %s\n", formatKeyValueStatus("Last 5 min", fmt.Sprintf("%d", agent.ErrorsLast5Min), "error")))
	}

	// Tags (limit to 10)
	if len(agent.Tags) > 0 {
		content.WriteString("\n" + formatSectionHeader("Tags") + "\n")
		limit := 10
		if len(agent.Tags) < limit {
			limit = len(agent.Tags)
		}
		for i := 0; i < limit; i++ {
			content.WriteString(fmt.Sprintf("  %s\n", subduedStyle.Render(agent.Tags[i])))
		}
		if len(agent.Tags) > limit {
			content.WriteString(fmt.Sprintf("  %s\n", subduedStyle.Render(fmt.Sprintf("... and %d more", len(agent.Tags)-limit))))
		}
	}

	return panelStyle.
		Width(width).
		Height(height).
		Render(content.String())
}

// renderIntakePanel renders the right panel showing backend connectivity
func (m model) renderIntakePanel(width, height int, isSelected bool) string {
	var content strings.Builder

	// Panel title with selection indicator
	title := "Intake"
	if isSelected {
		title = "▶ " + title
	}
	content.WriteString(titleStyle.Render(title) + "\n\n")

	// Connection status
	intake := m.status.Intake
	if intake.Connected {
		content.WriteString(fmt.Sprintf("%s\n\n", formatStatusIndicator("connected", 0)))
	} else {
		content.WriteString(fmt.Sprintf("%s\n\n", formatStatusIndicator("disconnected", 0)))
	}

	// API Key status
	content.WriteString(formatSectionHeader("API Key") + "\n")
	if intake.APIKeyInfo.Valid {
		content.WriteString(fmt.Sprintf("  %s\n", formatStatusIndicator("valid", 0)))
		if !intake.APIKeyInfo.LastValidated.IsZero() {
			lastValidated := time.Since(intake.APIKeyInfo.LastValidated)
			content.WriteString(fmt.Sprintf("  %s\n", formatKeyValue("Validated", fmt.Sprintf("%s ago", formatDuration(lastValidated)))))
		}
	} else {
		content.WriteString(fmt.Sprintf("  %s\n", formatStatusIndicator("invalid", 0)))
	}

	// Last flush
	if !intake.LastFlush.IsZero() {
		content.WriteString("\n" + formatSectionHeader("Last Flush") + "\n")
		timeSince := time.Since(intake.LastFlush)
		content.WriteString(fmt.Sprintf("  %s\n", formatKeyValue("Time", fmt.Sprintf("%s ago", formatDuration(timeSince)))))
	}

	// Retry queue
	if intake.RetryQueue > 0 {
		content.WriteString("\n" + formatSectionHeader("Retry Queue") + "\n")
		content.WriteString(fmt.Sprintf("  %s\n", formatKeyValueStatus("Size", fmt.Sprintf("%d", intake.RetryQueue), "warning")))
	}

	// Endpoints
	content.WriteString("\n" + formatSectionHeader("Endpoints") + "\n")
	for _, endpoint := range intake.Endpoints {
		var status string
		switch endpoint.Status {
		case "connected":
			status = formatStatusIndicator("ok", 0)
		case "error":
			status = formatStatusIndicator("error", 0)
		default:
			status = formatStatusIndicator("unknown", 0)
		}

		content.WriteString(fmt.Sprintf("  %s %s\n", status, highlightStyle.Render(endpoint.Name)))
		content.WriteString(fmt.Sprintf("    %s\n", subduedStyle.Render(truncate(endpoint.URL, 35))))

		if endpoint.LastError != "" && endpoint.Status == "error" {
			content.WriteString(fmt.Sprintf("    %s\n", errorStyle.Render(truncate(endpoint.LastError, 35))))
		}
	}

	return panelStyle.
		Width(width).
		Height(height).
		Render(content.String())
}

// Helper functions for formatting

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// formatLargeNumber formats large numbers with commas for readability
func formatLargeNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	if n < 1000000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	return fmt.Sprintf("%.1fB", float64(n)/1000000000)
}

// formatBytes formats bytes in a human-readable way
func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1fGB", float64(n)/(1024*1024*1024))
}

// truncate truncates a string to a maximum length and adds ellipsis if needed
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// renderLogsDetailView renders the full-screen logs detail view with two panels
// Left: List of log sources | Right: Streaming logs for selected source
func (m model) renderLogsDetailView() string {
	if m.status == nil {
		return "No data available"
	}

	// Calculate panel dimensions
	// Left panel: 40% of width for log sources list
	// Right panel: 60% of width for streaming logs
	leftWidth := int(float64(m.width) * 0.4)
	rightWidth := m.width - leftWidth - 4 // Account for borders and spacing
	contentHeight := m.height - 6         // Account for header and footer

	// Build left panel - log sources list
	leftPanel := m.renderLogSourcesList(leftWidth, contentHeight)

	// Build right panel - streaming logs
	rightPanel := m.renderStreamingLogs(rightWidth, contentHeight)

	// Combine panels horizontally
	panels := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPanel,
		rightPanel,
	)

	// Header
	header := titleStyle.Render(" LOGS DETAIL VIEW ")

	// Footer with instructions
	footer := subduedStyle.Render("↑/↓: Navigate | Esc: Back to main view | Q: Quit")

	// Combine all sections
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		panels,
		"",
		footer,
	)
}

// renderLogSourcesList renders the left panel with the list of log sources
func (m model) renderLogSourcesList(width, height int) string {
	var content strings.Builder

	logsStatus := m.status.Ingestion.Logs

	// Check if logs are enabled
	if !logsStatus.Enabled {
		content.WriteString(warningStyle.Render("⚠ Logs collection is DISABLED"))
		content.WriteString("\n\n")
		content.WriteString(subduedStyle.Render("Enable it in datadog.yaml with:\nlogs_enabled: true"))
	} else if len(logsStatus.Integrations) == 0 {
		content.WriteString(infoStyle.Render("✓ Logs collection is ENABLED"))
		content.WriteString("\n\n")
		content.WriteString(subduedStyle.Render("No log sources configured yet."))
	} else {
		content.WriteString(successStyle.Render(fmt.Sprintf("✓ ENABLED - %d source(s)", len(logsStatus.Integrations))))
		content.WriteString("\n\n")

		// Summary stats
		content.WriteString(subduedStyle.Render(fmt.Sprintf("Sources: %d\nBytes: %s\nErrors: %d",
			logsStatus.Sources,
			formatBytes(logsStatus.BytesProcessed),
			logsStatus.Errors)))
		content.WriteString("\n\n")

		// Separator
		content.WriteString(strings.Repeat("─", width-4))
		content.WriteString("\n\n")

		// List each log source
		for i, logSource := range logsStatus.Integrations {
			// Highlight selected source
			isSelected := i == m.selectedLogIdx

			// Source header
			var sourceName string
			if isSelected {
				sourceName = fmt.Sprintf("▶ %s", logSource.Name)
			} else {
				sourceName = fmt.Sprintf("  %s", logSource.Name)
			}

			// Status symbol
			statusSymbol := symbolInfo
			statusColor := subduedStyle
			switch logSource.Status {
			case "success":
				statusSymbol = symbolSuccess
				statusColor = successStyle
			case "error":
				statusSymbol = symbolError
				statusColor = errorStyle
			case "pending":
				statusSymbol = symbolRunning
				statusColor = warningStyle
			}

			content.WriteString(statusColor.Render(fmt.Sprintf("%s %s", statusSymbol, sourceName)))
			content.WriteString("\n")

			// Show details for selected source
			if isSelected {
				content.WriteString(subduedStyle.Render(fmt.Sprintf("   Type: %s", logSource.Type)))
				content.WriteString("\n")

				// Show inputs (files being tailed)
				if len(logSource.Inputs) > 0 {
					content.WriteString(subduedStyle.Render("   Files:"))
					content.WriteString("\n")
					for _, input := range logSource.Inputs {
						truncatedInput := truncate(input, width-10)
						content.WriteString(subduedStyle.Render(fmt.Sprintf("     • %s", truncatedInput)))
						content.WriteString("\n")
					}
				}

				// Show stats
				if len(logSource.Info) > 0 {
					content.WriteString(subduedStyle.Render("   Stats:"))
					content.WriteString("\n")
					for key, value := range logSource.Info {
						content.WriteString(subduedStyle.Render(fmt.Sprintf("     %s: %s", key, value)))
						content.WriteString("\n")
					}
				}
				content.WriteString("\n")
			}
		}
	}

	return panelStyle.
		Width(width).
		Height(height).
		Render(content.String())
}

// renderStreamingLogs renders the right panel with streaming logs
func (m model) renderStreamingLogs(width, height int) string {
	var content strings.Builder

	// Panel title
	if m.streamingSource != "" {
		content.WriteString(highlightStyle.Render(fmt.Sprintf("Streaming: %s", m.streamingSource)))
		content.WriteString("\n\n")
	} else {
		content.WriteString(subduedStyle.Render("Select a log source to view stream"))
		content.WriteString("\n\n")
	}

	// Show log lines
	if len(m.logLines) == 0 && m.streamingSource != "" {
		content.WriteString(subduedStyle.Render("Waiting for logs..."))
	} else {
		// Calculate how many lines we can show
		// Subtract 3 for title and spacing
		maxLines := height - 3
		if maxLines < 0 {
			maxLines = 0
		}

		// Show the last N lines
		startIdx := 0
		if len(m.logLines) > maxLines {
			startIdx = len(m.logLines) - maxLines
		}

		for _, line := range m.logLines[startIdx:] {
			// Truncate line if needed to fit width
			truncatedLine := truncate(line, width-4)
			content.WriteString(valueStyle.Render(truncatedLine))
			content.WriteString("\n")
		}
	}

	return panelStyle.
		Width(width).
		Height(height).
		Render(content.String())
}

// renderServicesPanel renders the leftmost panel showing services
func (m model) renderServicesPanel(width, height int, isSelected bool) string {
	var content strings.Builder

	// Panel title with selection indicator
	title := "Services"
	if isSelected {
		title = "▶ " + title
	}
	content.WriteString(titleStyle.Render(title) + "\n\n")

	if len(m.status.Services) == 0 {
		content.WriteString(subduedStyle.Render("No services detected"))
		content.WriteString("\n\n")
		content.WriteString(infoStyle.Render("Services will appear here\nwhen traces, metrics, or\nlogs are collected"))
	} else {
		content.WriteString(formatSectionHeader(fmt.Sprintf("Total: %d", len(m.status.Services))) + "\n")
		if isSelected {
			content.WriteString(subduedStyle.Render("(press Enter for details)") + "\n")
		}
		content.WriteString("\n")

		// Show up to 15 services in the panel
		limit := 15
		if len(m.status.Services) < limit {
			limit = len(m.status.Services)
		}

		for i := 0; i < limit; i++ {
			service := m.status.Services[i]

			// Format service name
			content.WriteString(highlightStyle.Render(service.Name))
			content.WriteString("\n")

			// Create mini sparkline visualization for each data type
			var indicators []string

			// Traces rate indicator
			if service.TracesRate > 0 {
				indicators = append(indicators, fmt.Sprintf("T:%s/s", formatRate(service.TracesRate)))
			}

			// Metrics rate indicator
			if service.MetricsRate > 0 {
				indicators = append(indicators, fmt.Sprintf("M:%s/s", formatRate(service.MetricsRate)))
			}

			// Logs rate indicator (bytes/s)
			if service.LogsRate > 0 {
				indicators = append(indicators, fmt.Sprintf("L:%s/s", formatBytesRate(service.LogsRate)))
			}

			if len(indicators) > 0 {
				content.WriteString("  ")
				content.WriteString(subduedStyle.Render(strings.Join(indicators, " ")))
				content.WriteString("\n")
			} else {
				content.WriteString("  ")
				content.WriteString(subduedStyle.Render("(no recent data)"))
				content.WriteString("\n")
			}

			content.WriteString("\n")
		}

		if len(m.status.Services) > limit {
			content.WriteString(subduedStyle.Render(fmt.Sprintf("... and %d more", len(m.status.Services)-limit)))
			content.WriteString("\n")
		}
	}

	return panelStyle.
		Width(width).
		Height(height).
		Render(content.String())
}

// renderServicesView renders the full-screen services detail view
func (m model) renderServicesView() string {
	if m.status == nil {
		return "No data available"
	}

	var content strings.Builder

	// Header
	header := titleStyle.Render(" SERVICES ")
	content.WriteString(header)
	content.WriteString("\n\n")

	if len(m.status.Services) == 0 {
		content.WriteString(infoStyle.Render("No services detected yet"))
		content.WriteString("\n\n")
		content.WriteString(subduedStyle.Render("Services will appear here when the agent receives:"))
		content.WriteString("\n")
		content.WriteString(subduedStyle.Render("  • Traces from APM instrumentation"))
		content.WriteString("\n")
		content.WriteString(subduedStyle.Render("  • Metrics from integration checks"))
		content.WriteString("\n")
		content.WriteString(subduedStyle.Render("  • Logs from configured sources"))
		content.WriteString("\n")
	} else {
		// Summary
		content.WriteString(successStyle.Render(fmt.Sprintf("✓ %d services detected", len(m.status.Services))))
		content.WriteString("\n\n")

		// Calculate how many rows we can show
		maxRows := m.height - 10 // Account for header, footer, spacing

		startIdx := 0
		endIdx := len(m.status.Services)
		if endIdx > maxRows {
			// If there are more services than we can display, show a window around the selected service
			windowSize := maxRows
			startIdx = m.selectedServiceIdx - windowSize/2
			if startIdx < 0 {
				startIdx = 0
			}
			endIdx = startIdx + windowSize
			if endIdx > len(m.status.Services) {
				endIdx = len(m.status.Services)
				startIdx = endIdx - windowSize
				if startIdx < 0 {
					startIdx = 0
				}
			}
		}

		// Header row
		content.WriteString(highlightStyle.Render("SERVICE"))
		content.WriteString(strings.Repeat(" ", 30))
		content.WriteString(highlightStyle.Render("TRACES"))
		content.WriteString(strings.Repeat(" ", 5))
		content.WriteString(highlightStyle.Render("METRICS"))
		content.WriteString(strings.Repeat(" ", 5))
		content.WriteString(highlightStyle.Render("LOGS"))
		content.WriteString("\n")
		content.WriteString(strings.Repeat("─", m.width-4))
		content.WriteString("\n")

		// Service rows
		for i := startIdx; i < endIdx; i++ {
			service := m.status.Services[i]
			isSelected := i == m.selectedServiceIdx

			// Selection indicator
			if isSelected {
				content.WriteString("▶ ")
			} else {
				content.WriteString("  ")
			}

			// Service name (truncate if too long)
			serviceName := truncate(service.Name, 35)
			if isSelected {
				content.WriteString(highlightStyle.Render(serviceName))
			} else {
				content.WriteString(valueStyle.Render(serviceName))
			}
			content.WriteString(strings.Repeat(" ", 37-len(serviceName)))

			// Traces rate with visual indicator
			tracesStr := formatRate(service.TracesRate) + "/s"
			if service.TracesRate > 0 {
				// Create a simple "sparkline" using block characters
				sparkline := createSparklineFromRate(service.TracesRate, 10)
				content.WriteString(fmt.Sprintf("%12s %s  ", tracesStr, sparkline))
			} else {
				content.WriteString(fmt.Sprintf("%12s %s  ", subduedStyle.Render("-"), subduedStyle.Render("          ")))
			}

			// Metrics rate with visual indicator
			metricsStr := formatRate(service.MetricsRate) + "/s"
			if service.MetricsRate > 0 {
				sparkline := createSparklineFromRate(service.MetricsRate, 10)
				content.WriteString(fmt.Sprintf("%12s %s  ", metricsStr, sparkline))
			} else {
				content.WriteString(fmt.Sprintf("%12s %s  ", subduedStyle.Render("-"), subduedStyle.Render("          ")))
			}

			// Logs rate with visual indicator (bytes/s)
			logsStr := formatBytesRate(service.LogsRate) + "/s"
			if service.LogsRate > 0 {
				sparkline := createSparklineFromRate(service.LogsRate, 10)
				content.WriteString(fmt.Sprintf("%12s %s", logsStr, sparkline))
			} else {
				content.WriteString(fmt.Sprintf("%12s %s", subduedStyle.Render("-"), subduedStyle.Render("          ")))
			}

			content.WriteString("\n")
		}
	}

	// Footer with instructions
	content.WriteString("\n")
	content.WriteString(subduedStyle.Render("↑/↓: Navigate | Esc: Back to main view | Q: Quit"))

	return baseStyle.Render(content.String())
}

// createSparklineFromRate creates a simple visual representation of a rate value
// using block characters to show relative magnitude
func createSparklineFromRate(rate float64, width int) string {
	if rate == 0 {
		return strings.Repeat("░", width)
	}

	// Use different block characters based on magnitude (logarithmic scale)
	var blocks string
	switch {
	case rate < 1:
		blocks = "▁"
	case rate < 10:
		blocks = "▂"
	case rate < 100:
		blocks = "▃"
	case rate < 1000:
		blocks = "▄"
	case rate < 10000:
		blocks = "▅"
	case rate < 100000:
		blocks = "▆"
	default:
		blocks = "▇"
	}

	// Color based on data type context
	// For now, use a simple blue color for all
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render(strings.Repeat(blocks, width))
	return styled
}

// formatRate formats a rate value with appropriate units (K, M, etc.)
func formatRate(rate float64) string {
	if rate == 0 {
		return "0"
	}
	if rate < 1 {
		return fmt.Sprintf("%.2f", rate)
	}
	if rate < 1000 {
		return fmt.Sprintf("%.1f", rate)
	}
	if rate < 1000000 {
		return fmt.Sprintf("%.1fK", rate/1000)
	}
	return fmt.Sprintf("%.1fM", rate/1000000)
}

// formatBytesRate formats bytes per second rate with appropriate units (KB/s, MB/s, GB/s)
func formatBytesRate(bytesPerSecond float64) string {
	if bytesPerSecond == 0 {
		return "0B"
	}
	if bytesPerSecond < 1024 {
		return fmt.Sprintf("%.0fB", bytesPerSecond)
	}
	if bytesPerSecond < 1024*1024 {
		return fmt.Sprintf("%.1fKB", bytesPerSecond/1024)
	}
	if bytesPerSecond < 1024*1024*1024 {
		return fmt.Sprintf("%.1fMB", bytesPerSecond/(1024*1024))
	}
	return fmt.Sprintf("%.1fGB", bytesPerSecond/(1024*1024*1024))
}
