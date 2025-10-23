// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Color palette for the TUI
// Using colors that work well across different terminal emulators
// with fallbacks for limited color support
var (
	// Status colors
	colorSuccess = lipgloss.AdaptiveColor{Light: "#00A000", Dark: "#00FF00"}
	colorWarning = lipgloss.AdaptiveColor{Light: "#D75F00", Dark: "#FFAF00"}
	colorError   = lipgloss.AdaptiveColor{Light: "#D70000", Dark: "#FF5555"}
	colorInfo    = lipgloss.AdaptiveColor{Light: "#005FD7", Dark: "#5FAFFF"}

	// Panel colors
	colorBorder     = lipgloss.AdaptiveColor{Light: "#8E8E8E", Dark: "#666666"}
	colorTitle      = lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}
	colorLabel      = lipgloss.AdaptiveColor{Light: "#4E4E4E", Dark: "#AAAAAA"}
	colorValue      = lipgloss.AdaptiveColor{Light: "#000000", Dark: "#E4E4E4"}
	colorSubdued    = lipgloss.AdaptiveColor{Light: "#767676", Dark: "#888888"}
	colorHighlight  = lipgloss.AdaptiveColor{Light: "#0087D7", Dark: "#87AFFF"}
	colorBackground = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1E1E1E"}
)

// Status symbols
const (
	symbolSuccess = "\u2713" // ✓
	symbolWarning = "\u26A0" // ⚠
	symbolError   = "\u2717" // ✗
	symbolInfo    = "\u2139" // ℹ
	symbolRunning = "\u25CF" // ●
)

// ASCII art for Agent Panel
const (
	// Datadog logo (simple DD text)
	datadogLogo = ` ╔═╗╔═╗
 ║ ║║ ║
 ║ ║║ ║
 ╚═╝╚═╝`

	// Bone animation character
	boneChar = "●"
)

// Animation constants
const (
	// Animation duration in milliseconds
	animationDuration = 1500 // 1.5 seconds total

	// Animation phase durations (as fraction of total)
	verticalPhase   = 0.7 // 70% of time for vertical movement (~1 second)
	horizontalPhase = 0.3 // 30% of time for horizontal movement (~0.5 seconds)

	// Horizontal offset for bone to reach endpoint (in columns)
	boneHorizOffset = 12

	// Flash duration for endpoint color changes
	flashDuration = 2000 // 2 seconds in milliseconds

	// Rate limiting for animations (milliseconds between animations per endpoint)
	animationRateLimit = 2000 // 2 seconds
)

// Endpoint status colors for Agent Panel
var (
	colorEndpointDefault = lipgloss.Color("240") // Gray
	colorEndpointSuccess = lipgloss.Color("15")  // White
	colorEndpointError   = lipgloss.Color("9")   // Red
)

// Style definitions for different UI elements
var (
	// Base styles
	baseStyle = lipgloss.NewStyle().
			Padding(0, 1)

	// Panel styles
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2).
			MarginRight(1)

	titleStyle = lipgloss.NewStyle().
			Foreground(colorTitle).
			Bold(true).
			Underline(true).
			MarginBottom(1)

	// Status indicator styles
	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(colorWarning).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	infoStyle = lipgloss.NewStyle().
			Foreground(colorInfo)

	// Text styles
	labelStyle = lipgloss.NewStyle().
			Foreground(colorLabel).
			Bold(true)

	valueStyle = lipgloss.NewStyle().
			Foreground(colorValue)

	subduedStyle = lipgloss.NewStyle().
			Foreground(colorSubdued).
			Italic(true)

	highlightStyle = lipgloss.NewStyle().
			Foreground(colorHighlight).
			Bold(true)

	// Section header style (within panels)
	sectionHeaderStyle = lipgloss.NewStyle().
				Foreground(colorHighlight).
				Bold(true).
				Underline(true).
				MarginTop(1).
				MarginBottom(0)

	// Footer styles (for keyboard shortcuts)
	footerStyle = lipgloss.NewStyle().
			Foreground(colorSubdued).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2)

	keyStyle = lipgloss.NewStyle().
			Foreground(colorHighlight).
			Bold(true)

	// Loading spinner style
	spinnerStyle = lipgloss.NewStyle().
			Foreground(colorInfo).
			Bold(true)

	// Error message style (full screen error)
	errorMessageStyle = lipgloss.NewStyle().
				Foreground(colorError).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorError).
				Padding(2, 4).
				MarginTop(2).
				Align(lipgloss.Center)
)

// formatStatusIndicator returns a styled status indicator with symbol
func formatStatusIndicator(status string, count int) string {
	switch status {
	case "ok", "healthy", "connected", "valid":
		if count > 0 {
			return successStyle.Render(symbolSuccess+" ") + valueStyle.Render(status)
		}
		return successStyle.Render(symbolSuccess + " " + status)
	case "warning":
		if count > 0 {
			return warningStyle.Render(symbolWarning+" ") + valueStyle.Render(status)
		}
		return warningStyle.Render(symbolWarning + " " + status)
	case "error", "unhealthy", "disconnected", "invalid":
		if count > 0 {
			return errorStyle.Render(symbolError+" ") + valueStyle.Render(status)
		}
		return errorStyle.Render(symbolError + " " + status)
	case "running":
		return infoStyle.Render(symbolRunning + " " + status)
	default:
		return subduedStyle.Render("- " + status)
	}
}

// formatKeyValue returns a styled key-value pair
func formatKeyValue(key, value string) string {
	return labelStyle.Render(key+": ") + valueStyle.Render(value)
}

// formatKeyValueStatus returns a styled key-value pair with status-based coloring
func formatKeyValueStatus(key, value, status string) string {
	valueStyled := valueStyle.Render(value)
	switch status {
	case "ok", "healthy", "connected", "valid":
		valueStyled = successStyle.Render(value)
	case "warning":
		valueStyled = warningStyle.Render(value)
	case "error", "unhealthy", "disconnected", "invalid":
		valueStyled = errorStyle.Render(value)
	}
	return labelStyle.Render(key+": ") + valueStyled
}

// formatSectionHeader returns a styled section header
func formatSectionHeader(title string) string {
	return sectionHeaderStyle.Render(title)
}

// formatCount returns a styled count with label
func formatCount(label string, count int, status string) string {
	countStr := valueStyle.Render(lipgloss.NewStyle().Render(lipgloss.NewStyle().Bold(true).Render(string(rune('0' + count%10)))))
	if count >= 10 {
		countStr = valueStyle.Render(lipgloss.NewStyle().Bold(true).Render(lipgloss.NewStyle().Render(string(rune('0'+count/10)))) + string(rune('0'+count%10)))
	}

	// Apply status color to the count
	switch status {
	case "error":
		countStr = errorStyle.Render(lipgloss.NewStyle().Bold(true).Render(string(count)))
	case "warning":
		countStr = warningStyle.Render(lipgloss.NewStyle().Bold(true).Render(string(count)))
	case "success", "ok":
		countStr = successStyle.Render(lipgloss.NewStyle().Bold(true).Render(string(count)))
	default:
		countStr = valueStyle.Render(lipgloss.NewStyle().Bold(true).Render(string(count)))
	}

	return labelStyle.Render(label+": ") + countStr
}
