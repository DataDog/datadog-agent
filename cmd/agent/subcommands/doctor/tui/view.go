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

	doctordef "github.com/DataDog/datadog-agent/comp/doctor/def"
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

// renderMainView renders the two-panel layout: services (left) and agent health (right)
func (m model) renderMainView() string {
	if m.status == nil {
		return "No data available"
	}

	// Calculate panel dimensions - split screen in half
	// Left panel: 50% for services list
	// Right panel: 50% for agent health
	leftPanelWidth := (m.width - 8) / 2 // Account for borders and spacing
	rightPanelWidth := (m.width - 8) / 2
	panelHeight := m.height - 6 // Leave space for header and footer

	// Render both panels with selection state
	servicesPanel := m.renderServicesPanel(leftPanelWidth, panelHeight, m.selectedPanel == 0)
	agentPanel := m.renderAgentPanel(rightPanelWidth, panelHeight, m.selectedPanel == 1)

	// Join panels horizontally
	panels := lipgloss.JoinHorizontal(
		lipgloss.Top,
		servicesPanel,
		agentPanel,
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
	case ServicesView:
		shortcuts = []string{
			keyStyle.Render("↑/↓/PgUp/PgDn/Home/End") + " navigate",
			keyStyle.Render("r") + " refresh",
			keyStyle.Render("esc") + " back",
			keyStyle.Render("q") + " quit",
		}
	default: // MainView
		shortcuts = []string{
			keyStyle.Render("←/→") + " switch panel",
			keyStyle.Render("↑/↓") + " scroll services",
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
	// Panel title with selection indicator
	title := "agent"
	if isSelected {
		title = "▶ " + title
	}

	// Build the panel content with animations
	content := m.renderAgentPanelContent(width, height-4) // Account for border and padding

	// Wrap in panel style
	return panelStyle.
		BorderForeground(colorBorder).
		Width(width).
		Height(height).
		Render(titleStyle.Render(title) + "\n\n" + content)
}

// renderServiceDetailsSection renders details for the currently selected service
func (m model) renderServiceDetailsSection() string {
	if m.status == nil || len(m.status.Services) == 0 {
		return subduedStyle.Render("No service selected")
	}

	// Get selected service
	selectedService := m.status.Services[m.selectedServiceIdx]
	serviceName := selectedService.Name
	displayServiceName := serviceName
	if serviceName == "" {
		displayServiceName = "other"
	}

	var result strings.Builder
	result.WriteString(highlightStyle.Render(fmt.Sprintf("Selected: %s", displayServiceName)))
	result.WriteString("\n\n")

	// Find checks for this service
	var serviceChecks []doctordef.CheckInfo
	for _, check := range m.status.Ingestion.Checks.CheckList {
		if check.Service == serviceName {
			serviceChecks = append(serviceChecks, check)
		}
	}

	// Find logs for this service
	var serviceLogs []doctordef.LogSource
	for _, log := range m.status.Ingestion.Logs.Integrations {
		if log.Service == serviceName {
			serviceLogs = append(serviceLogs, log)
		}
	}

	// Render checks
	result.WriteString(subduedStyle.Render("Checks:"))
	result.WriteString("\n")
	if len(serviceChecks) == 0 {
		result.WriteString(subduedStyle.Render("  No checks found"))
	} else {
		for _, check := range serviceChecks {
			statusSymbol := "✓"
			statusColor := successStyle
			switch check.Status {
			case "error":
				statusSymbol = "✗"
				statusColor = errorStyle
			case "warning":
				statusSymbol = "⚠"
				statusColor = warningStyle
			}
			result.WriteString(fmt.Sprintf("  %s %s\n",
				statusColor.Render(statusSymbol),
				valueStyle.Render(check.Name)))
		}
	}

	// Render logs
	result.WriteString("\n")
	result.WriteString(subduedStyle.Render("Logs:"))
	result.WriteString("\n")
	if len(serviceLogs) == 0 {
		result.WriteString(subduedStyle.Render("  No log sources found"))
	} else {
		for _, log := range serviceLogs {
			for _, input := range log.Inputs {
				result.WriteString(fmt.Sprintf("  • %s\n", valueStyle.Render(truncate(input, 40))))
			}
		}
	}

	return result.String()
}

// renderAgentPanelContent renders the complete Agent Panel with service details, logo, metadata, and connectivity
func (m model) renderAgentPanelContent(width, height int) string {
	if m.status == nil {
		return "No data available"
	}

	// Top section: Service details
	serviceDetails := m.renderServiceDetailsSection()

	// Middle section: Logo on left, Infos + Connectivity on right
	// Style the logo with purple color
	styledLogo := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Render(datadogLogo)

	// Render Infos section
	infosSection := m.renderAgentInfoSection()

	// Render connectivity section with separator
	connectivityHeight := 10 // Estimated height for connectivity display
	connectivity := m.renderConnectivitySection(width, connectivityHeight)
	connectivityWithSeparator := subduedStyle.Render("─── Connectivity ───") + "\n" + connectivity

	// Stack Infos and Connectivity vertically
	rightSide := lipgloss.JoinVertical(
		lipgloss.Left,
		infosSection,
		"\n",
		connectivityWithSeparator,
	)

	// Combine logo (left) and right side (infos + connectivity) horizontally
	middleSection := lipgloss.JoinHorizontal(
		lipgloss.Top,
		styledLogo,
		"  ", // Spacing
		rightSide,
	)

	// Combine all sections vertically: service details + middle section
	return lipgloss.JoinVertical(
		lipgloss.Left,
		serviceDetails,
		"\n",
		middleSection,
	)
}

// renderAgentInfoSection renders the agent info section with metadata (no logo)
func (m model) renderAgentInfoSection() string {
	if m.status == nil {
		return "No agent data available"
	}

	// Format uptime
	uptime := m.status.Agent.Uptime
	var uptimeStr string
	hours := int(uptime.Hours())
	minutes := int(uptime.Minutes()) % 60
	if hours > 0 {
		uptimeStr = fmt.Sprintf("%dh %dm", hours, minutes)
	} else {
		uptimeStr = fmt.Sprintf("%dm", minutes)
	}

	// Format tags (show first 3, then "+N more" if there are more)
	tagsStr := "none"
	if len(m.status.Agent.Tags) > 0 {
		maxTags := 3
		if len(m.status.Agent.Tags) <= maxTags {
			tagsStr = strings.Join(m.status.Agent.Tags, ", ")
		} else {
			displayedTags := strings.Join(m.status.Agent.Tags[:maxTags], ", ")
			remaining := len(m.status.Agent.Tags) - maxTags
			tagsStr = fmt.Sprintf("%s (+%d more)", displayedTags, remaining)
		}
	}

	// Format API key (masked)
	apiKeyStr := "not configured"
	if m.status.Intake.APIKeyInfo.APIKey != "" {
		apiKeyStr = m.status.Intake.APIKeyInfo.APIKey
	}

	// Build metadata text with "Infos" title
	var result strings.Builder
	result.WriteString(subduedStyle.Render("─── Infos ───") + "\n")
	result.WriteString(formatKeyValue("Uptime", uptimeStr) + "\n")
	result.WriteString(formatKeyValue("Version", m.status.Agent.Version) + "\n")
	result.WriteString(formatKeyValue("Hostname", m.status.Agent.Hostname) + "\n")
	result.WriteString(formatKeyValue("API Key", apiKeyStr) + "\n")
	result.WriteString(formatKeyValue("Tags", tagsStr))

	return result.String()
}

// renderConnectivitySection renders the connectivity section with endpoints and wire animations
func (m model) renderConnectivitySection(width, height int) string {
	if m.status == nil {
		return "No data available"
	}

	// Get list of endpoints and filter to only show those with recent activity
	allEndpoints := m.status.Intake.Endpoints
	if len(allEndpoints) == 0 {
		return "No endpoints available"
	}

	// Filter endpoints: only show those with activity in the last 30 seconds
	const activityTimeout = 30 * time.Second
	now := time.Now()
	endpoints := make([]doctordef.EndpointStatus, 0) // Will hold filtered endpoints

	for _, endpoint := range allEndpoints {
		lastActivity, hasActivity := m.lastActivityTime[endpoint.URL]
		if hasActivity && now.Sub(lastActivity) <= activityTimeout {
			endpoints = append(endpoints, endpoint)
		}
	}

	if len(endpoints) == 0 {
		return "No endpoints with recent activity"
	}

	var result strings.Builder

	// Render each endpoint with its wire and payloads
	for i, endpoint := range endpoints {
		if i > 0 {
			result.WriteString("\n")
		}

		// Determine endpoint dot color and URL color based on flash state
		var dotColor lipgloss.TerminalColor = colorEndpointDefault
		var urlColor lipgloss.TerminalColor = colorSubdued // Default: gray for inactive
		if flash, exists := m.endpointFlashState[endpoint.URL]; exists {
			dotColor = flash.color
			urlColor = flash.color // Flash color (white/red) when payload arrives
		}

		// Build the wire with payloads
		wire := m.renderWire(endpoint.URL)

		// // Format transaction counts (Success/Retry/Error)
		// countsStr := fmt.Sprintf("  (S:%s R:%s E:%s)",
		// 	successStyle.Render(formatLargeNumber(endpoint.SuccessCount)),
		// 	warningStyle.Render(formatLargeNumber(endpoint.RequeuedCount)),
		// 	errorStyle.Render(formatLargeNumber(endpoint.ErrorCount)),
		// )

		// Render: wire + dot + URL + counts
		result.WriteString(wire)
		result.WriteString(" ")
		result.WriteString(lipgloss.NewStyle().Foreground(dotColor).Render("●"))
		result.WriteString(" ")
		result.WriteString(lipgloss.NewStyle().Foreground(urlColor).Render(endpoint.URL))
		// result.WriteString(countsStr)

		// If we've reached height limit, stop rendering
		if i >= height-1 {
			break
		}
	}

	return result.String()
}

// renderWire builds a wire visualization with animated payloads
// Returns a styled string like "->->------" (10 characters)
// Arrows are colored based on type: white (normal) or yellow (retry)
// Wire dashes are white when active, gray when idle
func (m model) renderWire(endpointURL string) string {
	// Get payloads for this endpoint
	payloads, exists := m.endpointPayloads[endpointURL]
	hasActivePayloads := exists && len(payloads) > 0

	if !hasActivePayloads {
		// No payloads: render wire in gray (subdued)
		wire := strings.Repeat(wireChar, wireLength)
		return lipgloss.NewStyle().Foreground(colorSubdued).Render(wire)
	}

	// Build a map of positions to payloads for quick lookup
	payloadPositions := make(map[int]*payloadAnimation)
	for _, payload := range payloads {
		position := int(payload.progress * float64(wireLength))
		if position < 0 {
			position = 0
		}
		if position >= wireLength {
			position = wireLength - 1
		}
		payloadPositions[position] = payload
	}

	// Render wire character by character with individual colors
	var result strings.Builder
	for i := 0; i < wireLength; i++ {
		if payload, hasPayload := payloadPositions[i]; hasPayload {
			// Render arrow with color based on arrow type
			var arrowColor lipgloss.TerminalColor
			if payload.arrowType == "retry" {
				arrowColor = colorEndpointWarning // Yellow for retries
			} else {
				arrowColor = colorValue // White for normal sends
			}
			result.WriteString(lipgloss.NewStyle().Foreground(arrowColor).Render(payloadChar))
		} else {
			// Render dash in white (active wire)
			result.WriteString(lipgloss.NewStyle().Foreground(colorValue).Render(wireChar))
		}
	}

	return result.String()
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

// renderServicesPanel renders the left panel with services and their dot graphs
func (m model) renderServicesPanel(width, height int, isSelected bool) string {
	var content strings.Builder

	// Panel title with selection indicator (lowercase "services")
	title := "services"
	if isSelected {
		title = "▶ " + title
	}
	content.WriteString(titleStyle.Render(title) + "\n\n")

	// Check if there are any services
	totalServices := len(m.status.Services)
	// hasOther := m.otherTimeSeries != nil && (len(m.otherTimeSeries.metrics.values) > 0 ||
	// 	len(m.otherTimeSeries.logs.values) > 0 ||
	// 	len(m.otherTimeSeries.traces.values) > 0)

	// if totalServices == 0 && !hasOther {
	if totalServices == 0 {
		content.WriteString(subduedStyle.Render("No services detected yet\n\n"))
		content.WriteString(infoStyle.Render("Services will appear here\nwhen traces, metrics, or\nlogs are collected"))
	} else {
		// Calculate how many services we can display based on panel height
		// Each service box takes 6 lines (horizontal graphs) + 2 lines (top/bottom border) = 8 lines
		linesPerService := 8
		availableHeight := height - 5 // Account for title and padding
		maxVisibleServices := availableHeight / linesPerService
		if maxVisibleServices < 1 {
			maxVisibleServices = 1
		}

		// Calculate scroll window
		startIdx := m.scrollOffset
		endIdx := startIdx + maxVisibleServices
		if endIdx > totalServices { // +1 for "other"
			endIdx = totalServices
		}

		// Render each visible service
		for i := startIdx; i < endIdx; i++ {
			var serviceName string
			var ts *serviceTimeSeries
			isServiceSelected := isSelected && i == m.selectedServiceIdx
			service := m.status.Services[i]
			isOther := service.Name == ""

			// Regular service
			serviceName = service.Name
			if isOther {
				serviceName = "other"
			}
			ts = m.serviceTimeSeries[service.Name]

			// Render service with compact layout and border
			serviceContent := m.renderCompactServiceBox(serviceName, ts, isServiceSelected, isOther)

			// Create border style
			borderStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder).
				Padding(0, 1)
				// MarginBottom(1)

			// Highlight border if selected
			if isServiceSelected {
				borderStyle = borderStyle.BorderForeground(colorHighlight)
			}

			borderedContent := borderStyle.Render(serviceContent)
			content.WriteString(borderedContent)
			content.WriteString("\n")
		}

		// Scroll indicator if needed
		if totalServices > maxVisibleServices {
			remaining := (totalServices) - endIdx
			if remaining > 0 {
				content.WriteString(subduedStyle.Render(fmt.Sprintf("... %d more ↓", remaining)))
			}
		}
	}

	return panelStyle.
		Width(width).
		Height(height).
		Render(content.String())
}

// renderCompactServiceBox renders a single service with dot graphs in a compact format
// Layout: service name on left, 3 graphs horizontally on right (6 rows total)
func (m model) renderCompactServiceBox(serviceName string, ts *serviceTimeSeries, isSelected bool, isOther bool) string {
	// Service name styling
	nameStyle := lipgloss.NewStyle().
		Foreground(colorHighlight).
		Bold(isSelected)

	displayName := serviceName
	if isSelected {
		displayName = "▶ " + displayName
	} else {
		displayName = "  " + displayName
	}

	// Add caption for "other" service
	if isOther {
		displayName += " " + subduedStyle.Render("(unattributed)")
	}

	styledName := nameStyle.Render(displayName)

	// Render dot graphs if we have time series data
	var graphsContent string
	if ts != nil && len(ts.metrics.values) > 0 {
		graphsContent = renderServiceDotGraphs(ts)
	} else {
		// No data yet - show placeholder (6 lines to match graph height)
		graphsContent = subduedStyle.Render("(no data)\n\n\n\n\n")
	}

	// Combine name and graphs side by side
	// Name should be vertically centered with the 6-row graphs
	nameLines := []string{
		styledName,
		"",
		"",
		"",
		"",
		"",
	}

	graphLines := strings.Split(graphsContent, "\n")
	// Pad graph lines to ensure we have 6 lines
	for len(graphLines) < 6 {
		graphLines = append(graphLines, "")
	}

	// Calculate service name column width (for padding)
	serviceNameWidth := 25

	// Combine line by line
	var result strings.Builder
	for i := 0; i < 6; i++ {
		// Pad service name column
		nameLine := ""
		if i < len(nameLines) {
			nameLine = nameLines[i]
		}
		paddedName := padRightWithWidth(nameLine, serviceNameWidth)

		// Graph line
		graphLine := ""
		if i < len(graphLines) {
			graphLine = graphLines[i]
		}

		result.WriteString(paddedName)
		result.WriteString(graphLine)
		if i < 5 { // Don't add newline after last line
			result.WriteString("\n")
		}
	}

	return result.String()
}

// padRightWithWidth pads a string to the specified width (accounts for ANSI color codes)
func padRightWithWidth(s string, width int) string {
	// Strip ANSI codes to measure actual visible length
	visibleLen := lipgloss.Width(s)
	if visibleLen >= width {
		return s
	}
	padding := strings.Repeat(" ", width-visibleLen)
	return s + padding
}

// renderServicesView renders the full-screen services detail view with dot graphs
func (m model) renderServicesView() string {
	if m.status == nil {
		return "No data available"
	}

	var content strings.Builder

	// Title: "services" in lowercase, monospace
	title := lipgloss.NewStyle().
		Foreground(colorTitle).
		Bold(true).
		Render("services")
	content.WriteString(title)
	content.WriteString("\n\n")

	// Check if there are any services
	totalServices := len(m.status.Services)
	// hasOther := m.otherTimeSeries != nil && (len(m.otherTimeSeries.metrics.values) > 0 ||
	// 	len(m.otherTimeSeries.logs.values) > 0 ||
	// 	len(m.otherTimeSeries.traces.values) > 0)

	// if totalServices == 0 && !hasOther {
	if totalServices == 0 {
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
		// Calculate dimensions for dot graphs
		// Each service takes approximately 15 lines (name + padding + 3 graphs * 4 rows + spacing)
		linesPerService := 15
		availableHeight := m.height - 6 // Leave space for title and footer
		maxVisibleServices := availableHeight / linesPerService

		// Calculate scroll window
		startIdx := m.scrollOffset
		endIdx := startIdx + maxVisibleServices
		if endIdx > totalServices { // +1 for "other"
			endIdx = totalServices
		}

		// Calculate dot graph width based on terminal width
		// Formula: (terminal_width - service_name_width - padding - borders) / columns
		dotGraphWidth := (m.width - 40) / 2 // Reserve 40 chars for service name and padding
		if dotGraphWidth < 10 {
			dotGraphWidth = 10 // Minimum width
		}
		if dotGraphWidth > 60 {
			dotGraphWidth = 60 // Maximum width (2 minutes at 2s intervals)
		}

		// Render each service
		for i := startIdx; i < endIdx; i++ {
			var serviceName string
			var ts *serviceTimeSeries
			isSelected := i == m.selectedServiceIdx
			isOther := m.status.Services[i].Name == ""

			// Regular service
			service := m.status.Services[i]
			serviceName = service.Name
			if serviceName == "" {
				serviceName = "other"
			}
			ts = m.serviceTimeSeries[serviceName]

			// Render the service box
			serviceBox := m.renderServiceBox(serviceName, ts, dotGraphWidth, isSelected, isOther)
			content.WriteString(serviceBox)
			content.WriteString("\n")
		}

		// Scroll indicator
		if totalServices+1 > maxVisibleServices {
			scrollInfo := fmt.Sprintf("Showing %d-%d of %d services", startIdx+1, endIdx, totalServices+1)
			content.WriteString("\n")
			content.WriteString(subduedStyle.Render(scrollInfo))
		}
	}

	// Footer with instructions
	content.WriteString("\n")
	footer := subduedStyle.Render("↑/↓/PgUp/PgDn/Home/End: Navigate | r: Refresh | Enter: Details | Esc: Back | Q: Quit")
	content.WriteString(footer)

	return baseStyle.Render(content.String())
}

// renderServiceBox renders a single service with its dot graphs in a rounded rectangle
func (m model) renderServiceBox(serviceName string, ts *serviceTimeSeries, dotGraphWidth int, isSelected bool, isOther bool) string {
	var content strings.Builder

	// Service name styling
	nameStyle := lipgloss.NewStyle().
		Foreground(colorHighlight).
		Bold(isSelected)

	if isSelected {
		serviceName = "▶ " + serviceName
	} else {
		serviceName = "  " + serviceName
	}

	// Add caption for "other" service
	if isOther {
		serviceName += " " + subduedStyle.Render("(unattributed activity)")
	}

	content.WriteString(nameStyle.Render(serviceName))
	content.WriteString("\n\n")

	// Render dot graphs if we have time series data
	if ts != nil {
		dotGraphs := renderServiceDotGraphs(ts)
		content.WriteString(dotGraphs)
	} else {
		// No data yet
		content.WriteString(subduedStyle.Render("(no historical data yet)"))
	}

	// Create the box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2).
		MarginBottom(1)

	if isSelected {
		// Brighter border for selected service
		boxStyle = boxStyle.BorderForeground(colorHighlight)
	}

	return boxStyle.Render(content.String())
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
