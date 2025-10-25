// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// dotGraphColors defines the colors for metrics, logs, and traces
var (
	colorMetrics  = lipgloss.Color("12")  // Blue
	colorLogs     = lipgloss.Color("220") // Yellow
	colorTraces   = lipgloss.Color("141") // Purple
	colorInactive = lipgloss.Color("240") // Dark gray
)

// timeSeriesData represents activity for a single telemetry type over time
type timeSeriesData struct {
	values []float64 // Rolling window of values (newest at end)
	maxLen int       // Maximum number of time buckets to store
}

// newTimeSeriesData creates a new time series buffer
func newTimeSeriesData(maxLen int) *timeSeriesData {
	return &timeSeriesData{
		values: make([]float64, 0, maxLen),
		maxLen: maxLen,
	}
}

// add appends a new value to the time series, removing oldest if necessary
func (ts *timeSeriesData) add(value float64) {
	ts.values = append(ts.values, value)
	if len(ts.values) > ts.maxLen {
		ts.values = ts.values[1:] // Shift left, remove oldest
	}
}

// getAverage calculates the average of all values in the window
func (ts *timeSeriesData) getAverage() float64 {
	if len(ts.values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range ts.values {
		sum += v
	}
	return sum / float64(len(ts.values))
}

// getMax returns the maximum value in the window
func (ts *timeSeriesData) getMax() float64 {
	if len(ts.values) == 0 {
		return 0
	}
	max := ts.values[0]
	for _, v := range ts.values {
		if v > max {
			max = v
		}
	}
	return max
}

// serviceTimeSeries tracks time series data for a single service across all telemetry types
type serviceTimeSeries struct {
	metrics *timeSeriesData
	logs    *timeSeriesData
	traces  *timeSeriesData
}

// newServiceTimeSeries creates a new service time series tracker
func newServiceTimeSeries(maxLen int) *serviceTimeSeries {
	return &serviceTimeSeries{
		metrics: newTimeSeriesData(maxLen),
		logs:    newTimeSeriesData(maxLen),
		traces:  newTimeSeriesData(maxLen),
	}
}

// renderDotGraph renders a 6-row dot graph for a single telemetry type
// width is the number of time bucket columns to display
// values is the time series data to visualize
// color is the color for active cells
// avg and max are used to determine the fill threshold for each column
// Values are right-aligned: newest values appear on the right, padding left with empty cells if needed
func renderDotGraph(width int, values []float64, color lipgloss.Color, avg, max float64) string {
	const rows = 6

	// Build the graph row by row (top to bottom)
	var lines []string
	for row := 0; row < rows; row++ {
		var line strings.Builder

		// Calculate how many values we have and how many empty columns we need on the left
		numValues := len(values)
		emptyColumns := 0
		startIdx := 0

		if numValues < width {
			// Buffer not full yet - pad left with empty columns, align values to the right
			emptyColumns = width - numValues
			startIdx = 0
		} else {
			// Buffer full - show the last 'width' values
			startIdx = numValues - width
		}

		// Render empty columns on the left (padding)
		for col := 0; col < emptyColumns; col++ {
			line.WriteString(lipgloss.NewStyle().Foreground(colorInactive).Render("·"))
		}

		// Render actual values on the right
		for col := 0; col < numValues && col < width; col++ {
			valueIdx := startIdx + col
			var cellActive bool

			if valueIdx < len(values) {
				value := values[valueIdx]
				cellActive = shouldFillCell(row, value, avg, max)
			}

			// Render cell
			if cellActive {
				// Use colored filled circle
				line.WriteString(lipgloss.NewStyle().Foreground(color).Render("●"))
			} else {
				// Use gray dot
				line.WriteString(lipgloss.NewStyle().Foreground(colorInactive).Render("·"))
			}
		}

		lines = append(lines, line.String())
	}

	return strings.Join(lines, "\n")
}

// shouldFillCell determines if a cell should be filled based on row position and value
// 6 rows provide fine-grained visualization:
// Row 0 (top): >= max
// Row 1: >= 5/6 of max
// Row 2: >= 4/6 of max
// Row 3: >= 3/6 of max
// Row 4: >= 2/6 of max
// Row 5 (bottom): > 0
func shouldFillCell(row int, value, avg, max float64) bool {
	if value == 0 {
		return false
	}

	// Use max for thresholds to show relative magnitude
	if max == 0 {
		return value > 0
	}

	switch row {
	case 0: // Top row - only filled if value is at max
		return value >= max
	case 1: // Second from top - filled if value >= 5/6 of max
		return value >= (max * 5.0 / 6.0)
	case 2: // Third from top - filled if value >= 4/6 of max
		return value >= (max * 4.0 / 6.0)
	case 3: // Middle - filled if value >= 3/6 (half) of max
		return value >= (max * 3.0 / 6.0)
	case 4: // Second from bottom - filled if value >= 2/6 of max
		return value >= (max * 2.0 / 6.0)
	case 5: // Bottom row - filled if value > 0
		return value > 0
	default:
		return false
	}
}

// renderServiceDotGraphs renders all three dot graphs (metrics, logs, traces) horizontally side-by-side
// with a unified info block on the left showing all metrics info
// graphWidth specifies the width of each individual graph (number of time columns)
func renderServiceDotGraphs(sts *serviceTimeSeries, graphWidth, infoBlockWidth int) string {
	// Get the visible window for each telemetry type
	// metricsWindow := getVisibleWindow(sts.metrics.values, graphWidth)
	// logsWindow := getVisibleWindow(sts.logs.values, graphWidth)
	// tracesWindow := getVisibleWindow(sts.traces.values, graphWidth)

	// Calculate averages and max for visible window only
	metricsAvg := getWindowAverage(sts.metrics.values)
	metricsMax := getWindowMax(sts.metrics.values)
	metricsCurrent := getCurrentValue(sts.metrics.values)

	logsAvg := getWindowAverage(sts.logs.values)
	logsMax := getWindowMax(sts.logs.values)
	logsCurrent := getCurrentValue(sts.logs.values)

	tracesAvg := getWindowAverage(sts.traces.values)
	tracesMax := getWindowMax(sts.traces.values)
	tracesCurrent := getCurrentValue(sts.traces.values)

	// Render each graph (6 rows each)
	var metricsGraph, logsGraph, tracesGraph string
	if graphWidth > 0 {
		metricsGraph = renderDotGraph(graphWidth, sts.metrics.values, colorMetrics, metricsAvg, metricsMax)
		logsGraph = renderDotGraph(graphWidth, sts.logs.values, colorLogs, logsAvg, logsMax)
		tracesGraph = renderDotGraph(graphWidth, sts.traces.values, colorTraces, tracesAvg, tracesMax)
	}

	// Create unified info block (6 lines) showing all metrics
	infoBlock := formatUnifiedInfoBlock(metricsCurrent, metricsAvg, logsCurrent, logsAvg, tracesCurrent, tracesAvg)

	// Split graphs into lines
	metricsLines := strings.Split(metricsGraph, "\n")
	logsLines := strings.Split(logsGraph, "\n")
	tracesLines := strings.Split(tracesGraph, "\n")
	infoLines := strings.Split(infoBlock, "\n")

	// Ensure we have 6 lines for each
	for len(metricsLines) < 6 {
		metricsLines = append(metricsLines, "")
	}
	for len(logsLines) < 6 {
		logsLines = append(logsLines, "")
	}
	for len(tracesLines) < 6 {
		tracesLines = append(tracesLines, "")
	}
	for len(infoLines) < 6 {
		infoLines = append(infoLines, "")
	}

	// Combine: info block on left, then 3 graphs horizontally on right
	var result strings.Builder

	for i := 0; i < 6; i++ {
		// Info block (left side)
		paddedInfo := padRight(infoLines[i], infoBlockWidth)
		result.WriteString(paddedInfo)
		result.WriteString(" ") // Space after info block

		// Metrics graph
		result.WriteString(metricsLines[i])
		result.WriteString("  ") // Spacing between graphs

		// Logs graph
		result.WriteString(logsLines[i])
		result.WriteString("  ") // Spacing between graphs

		// Traces graph
		result.WriteString(tracesLines[i])

		if i < 5 { // Don't add newline after last line
			result.WriteString("\n")
		}
	}

	return result.String()
}

// getCurrentValue returns the most recent value from the time series
func getCurrentValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}

// getVisibleWindow returns the last N values from the time series (the visible window)
func getVisibleWindow(values []float64, windowSize int) []float64 {
	// If the window is null, return all values
	if windowSize <= 0 {
		return values
	}
	if len(values) == 0 {
		return []float64{}
	}
	if len(values) <= windowSize {
		return values
	}
	return values[len(values)-windowSize:]
}

// getWindowAverage calculates the average of values in the window
func getWindowAverage(window []float64) float64 {
	if len(window) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range window {
		sum += v
	}
	return sum / float64(len(window))
}

// getWindowMax returns the maximum value in the window
func getWindowMax(window []float64) float64 {
	if len(window) == 0 {
		return 0
	}
	max := window[0]
	for _, v := range window {
		if v > max {
			max = v
		}
	}
	return max
}

// formatGraphLabel creates a 2-line label for a graph showing current value and average
// Format: "Name: 600/s"  (line 1)
//
//	"~200"         (line 2)
func formatGraphLabel(name string, current, avg float64, isBytes bool) string {
	var currentStr, avgStr string

	if isBytes {
		// Format as bytes
		currentStr = formatBytesPerSecond(current)
		avgStr = formatBytes(int64(avg))
	} else {
		// Format as count
		currentStr = formatRate(current)
		avgStr = formatRate(avg)
	}

	line1 := lipgloss.NewStyle().Foreground(colorLabel).Render(name+": ") +
		lipgloss.NewStyle().Foreground(colorValue).Render(currentStr+"/s")
	line2 := lipgloss.NewStyle().Foreground(colorSubdued).Render("~" + avgStr)

	return line1 + "\n" + line2
}

// combineGraphWithLabel combines a 2-line label with a 6-row graph horizontally
// Label appears at top, rest is empty padding
func combineGraphWithLabel(label string, graph string) string {
	labelLines := strings.Split(label, "\n")
	graphLines := strings.Split(graph, "\n")

	// Ensure we have exactly 6 lines for label (2 lines of text + 4 empty)
	for len(labelLines) < 6 {
		labelLines = append(labelLines, "")
	}
	// Ensure we have exactly 6 lines for graph
	for len(graphLines) < 6 {
		graphLines = append(graphLines, "")
	}

	// Pad label to consistent width (e.g., 20 characters)
	labelWidth := 20
	var result strings.Builder

	for i := 0; i < 6; i++ {
		paddedLabel := padRight(labelLines[i], labelWidth)
		result.WriteString(paddedLabel)
		result.WriteString(" ")
		result.WriteString(graphLines[i])
		if i < 5 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// padRight pads a string to the specified width (accounts for ANSI color codes)
func padRight(s string, width int) string {
	// Strip ANSI codes to measure actual visible length
	visibleLen := lipgloss.Width(s)
	if visibleLen >= width {
		return s
	}
	padding := strings.Repeat(" ", width-visibleLen)
	return s + padding
}

// formatBytesPerSecond formats bytes per second rate
func formatBytesPerSecond(bytesPerSecond float64) string {
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

// formatBytes formats bytes without the /s suffix
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

// formatRate formats a rate value
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

// formatUnifiedInfoBlock creates a 6-line info block showing all metrics info stacked
// Lines 0-1: Metrics (current + avg)
// Lines 2-3: Logs (current + avg)
// Lines 4-5: Traces (current + avg)
func formatUnifiedInfoBlock(metricsCurr, metricsAvg, logsCurr, logsAvg, tracesCurr, tracesAvg float64) string {
	// Format metrics info (2 lines)
	metricsLine1 := lipgloss.NewStyle().Foreground(colorLabel).Render("Metrics: ") +
		lipgloss.NewStyle().Foreground(colorValue).Render(formatRate(metricsCurr)+"/s")
	metricsLine2 := lipgloss.NewStyle().Foreground(colorSubdued).Render("~" + formatRate(metricsAvg) + "/s")

	// Format logs info (2 lines)
	logsLine1 := lipgloss.NewStyle().Foreground(colorLabel).Render("Logs: ") +
		lipgloss.NewStyle().Foreground(colorValue).Render(formatBytesPerSecond(logsCurr))
	logsLine2 := lipgloss.NewStyle().Foreground(colorSubdued).Render("~" + formatBytes(int64(logsAvg)))

	// Format traces info (2 lines)
	tracesLine1 := lipgloss.NewStyle().Foreground(colorLabel).Render("Traces: ") +
		lipgloss.NewStyle().Foreground(colorValue).Render(formatRate(tracesCurr)+"/s")
	tracesLine2 := lipgloss.NewStyle().Foreground(colorSubdued).Render("~" + formatRate(tracesAvg) + "/s")

	// Combine into 6 lines
	return metricsLine1 + "\n" +
		metricsLine2 + "\n" +
		logsLine1 + "\n" +
		logsLine2 + "\n" +
		tracesLine1 + "\n" +
		tracesLine2
}
