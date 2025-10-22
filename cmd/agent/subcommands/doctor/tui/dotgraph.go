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

// renderDotGraph renders a 2-row dot graph for a single telemetry type
// width is the number of time bucket columns to display
// values is the time series data to visualize
// color is the color for active cells
// avg and max are used to determine the fill threshold for each column
func renderDotGraph(width int, values []float64, color lipgloss.Color, avg, max float64) string {
	const rows = 2

	// Build the graph row by row (top to bottom)
	var lines []string
	for row := 0; row < rows; row++ {
		var line strings.Builder

		// Determine how many values we can show based on width
		startIdx := 0
		if len(values) > width {
			startIdx = len(values) - width
		}

		// Fill cells for each time bucket column
		for col := 0; col < width; col++ {
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
// Row 0 is top (only filled if value >= ½ avg)
// Row 1 is bottom (filled if value > 0)
func shouldFillCell(row int, value, avg, max float64) bool {
	if value == 0 {
		return false
	}

	// Threshold based on average
	threshold := avg * 0.5 // ½ average

	switch row {
	case 0: // Top row - filled if value >= ½ avg
		return value >= threshold
	case 1: // Bottom row - filled if value > 0
		return value > 0
	default:
		return false
	}
}

// renderServiceDotGraphs renders all three dot graphs (metrics, logs, traces) vertically stacked
// with labels showing current value and average
func renderServiceDotGraphs(width int, sts *serviceTimeSeries) string {
	// Calculate averages and current values for each telemetry type
	metricsAvg := sts.metrics.getAverage()
	metricsMax := sts.metrics.getMax()
	metricsCurrent := getCurrentValue(sts.metrics.values)

	logsAvg := sts.logs.getAverage()
	logsMax := sts.logs.getMax()
	logsCurrent := getCurrentValue(sts.logs.values)

	tracesAvg := sts.traces.getAverage()
	tracesMax := sts.traces.getMax()
	tracesCurrent := getCurrentValue(sts.traces.values)

	// Render each graph
	metricsGraph := renderDotGraph(width, sts.metrics.values, colorMetrics, metricsAvg, metricsMax)
	logsGraph := renderDotGraph(width, sts.logs.values, colorLogs, logsAvg, logsMax)
	tracesGraph := renderDotGraph(width, sts.traces.values, colorTraces, tracesAvg, tracesMax)

	// Create labels for each graph
	metricsLabel := formatGraphLabel("Metrics", metricsCurrent, metricsAvg, false)
	logsLabel := formatGraphLabel("Logs", logsCurrent, logsAvg, true)
	tracesLabel := formatGraphLabel("Traces", tracesCurrent, tracesAvg, false)

	// Combine labels and graphs (label is 2 rows tall, same as graph)
	metricsLine := combineGraphWithLabel(metricsLabel, metricsGraph)
	logsLine := combineGraphWithLabel(logsLabel, logsGraph)
	tracesLine := combineGraphWithLabel(tracesLabel, tracesGraph)

	return metricsLine + "\n" + logsLine + "\n" + tracesLine
}

// getCurrentValue returns the most recent value from the time series
func getCurrentValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
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

// combineGraphWithLabel combines a 2-line label with a 2-row graph horizontally
func combineGraphWithLabel(label string, graph string) string {
	labelLines := strings.Split(label, "\n")
	graphLines := strings.Split(graph, "\n")

	// Ensure we have exactly 2 lines for both
	for len(labelLines) < 2 {
		labelLines = append(labelLines, "")
	}
	for len(graphLines) < 2 {
		graphLines = append(graphLines, "")
	}

	// Pad label to consistent width (e.g., 20 characters)
	labelWidth := 20
	paddedLabel1 := padRight(labelLines[0], labelWidth)
	paddedLabel2 := padRight(labelLines[1], labelWidth)

	return paddedLabel1 + " " + graphLines[0] + "\n" +
		paddedLabel2 + " " + graphLines[1]
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
