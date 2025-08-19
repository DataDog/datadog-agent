// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealthrecommendation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"

	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// runHealthRecommendation runs health checks from all subcomponents and displays the issues found
func runHealthRecommendation(_ log.Component, healthPlatform healthplatform.Component, cliParams *cliParams) error {
	ctx := context.Background()

	// Start the health platform if it's not already running
	if err := healthPlatform.Start(ctx); err != nil {
		return fmt.Errorf("failed to start health platform: %w", err)
	}
	defer func() {
		if err := healthPlatform.Stop(); err != nil {
			// Note: We can't use the log component here as it's not in scope
			// Just ignore the error for now as this is a cleanup operation
		}
	}()

	// Wait a moment for health checks to run
	time.Sleep(2 * time.Second)

	// Collect all issues
	allIssues := collectAllIssues(ctx, healthPlatform)

	// Filter issues based on flags
	filteredIssues := filterIssues(allIssues, cliParams)

	// Display results
	if cliParams.jsonOutput {
		return displayJSONResults(filteredIssues)
	}
	return displayTextResults(filteredIssues, cliParams.verbose)
}

// collectAllIssues collects issues from the main health platform and all sub-components
func collectAllIssues(_ context.Context, healthPlatform healthplatform.Component) []healthplatform.Issue {
	// Get issues from the main health platform
	mainIssues := healthPlatform.ListIssues()

	// For now, we'll just return the main issues
	// In a full implementation, we would also collect from sub-components
	// by calling their CheckHealth methods
	return mainIssues
}

// filterIssues filters issues based on the provided criteria
func filterIssues(issues []healthplatform.Issue, cliParams *cliParams) []healthplatform.Issue {
	if cliParams.severity == "" && cliParams.location == "" && cliParams.integration == "" {
		return issues
	}

	var filtered []healthplatform.Issue
	for _, issue := range issues {
		if cliParams.severity != "" && issue.Severity != cliParams.severity {
			continue
		}
		if cliParams.location != "" && issue.Location != cliParams.location {
			continue
		}
		if cliParams.integration != "" && issue.IntegrationFeature != cliParams.integration {
			continue
		}
		filtered = append(filtered, issue)
	}

	return filtered
}

// displayJSONResults outputs the results in JSON format
func displayJSONResults(issues []healthplatform.Issue) error {
	result := map[string]interface{}{
		"timestamp":    time.Now().Unix(),
		"total_issues": len(issues),
		"issues":       issues,
	}

	encoder := json.NewEncoder(color.Output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// displayTextResults outputs the results in a human-readable text format
func displayTextResults(issues []healthplatform.Issue, verbose bool) error {
	if len(issues) == 0 {
		fmt.Fprintf(color.Output, "%s No health issues found. The agent appears to be healthy!\n",
			color.GreenString("✅"))
		return nil
	}

	// Group issues by severity
	issuesBySeverity := make(map[string][]healthplatform.Issue)
	for _, issue := range issues {
		issuesBySeverity[issue.Severity] = append(issuesBySeverity[issue.Severity], issue)
	}

	// Define severity order for display
	severityOrder := []string{"critical", "high", "medium", "low"}

	fmt.Fprintf(color.Output, "%s Health Check Results - Found %d issue(s)\n\n",
		color.CyanString("🔍"), len(issues))

	for _, sev := range severityOrder {
		if issues, exists := issuesBySeverity[sev]; exists {
			emoji := getSeverityEmoji(sev)
			severityColor := getSeverityColor(sev)

			fmt.Fprintf(color.Output, "%s %s Severity Issues (%d):\n",
				emoji, severityColor("%s", sev), len(issues))
			fmt.Fprintln(color.Output, strings.Repeat("-", 50))

			for _, issue := range issues {
				fmt.Fprintf(color.Output, "  ID: %s\n", issue.ID)
				fmt.Fprintf(color.Output, "  Description: %s\n", issue.Description)
				fmt.Fprintf(color.Output, "  Location: %s\n", issue.Location)
				fmt.Fprintf(color.Output, "  Integration: %s\n", issue.IntegrationFeature)
				if issue.Extra != "" {
					fmt.Fprintf(color.Output, "  Details: %s\n", issue.Extra)
				}
				if verbose {
					fmt.Fprintf(color.Output, "  Severity: %s\n", issue.Severity)
				}
				fmt.Println()
			}
		}
	}

	// Display summary
	fmt.Fprintln(color.Output, color.CyanString("📊 Summary:"))
	fmt.Fprintf(color.Output, "  Total Issues: %d\n", len(issues))
	for sev, count := range issuesBySeverity {
		emoji := getSeverityEmoji(sev)
		severityColor := getSeverityColor(sev)
		fmt.Fprintf(color.Output, "  %s %s: %d\n", emoji, severityColor("%s", sev), len(count))
	}

	return nil
}

// getSeverityEmoji returns the appropriate emoji for a severity level
func getSeverityEmoji(severity string) string {
	switch severity {
	case "critical":
		return "🚨"
	case "high":
		return "⚠️"
	case "medium":
		return "🔶"
	case "low":
		return "ℹ️"
	default:
		return "❓"
	}
}

// getSeverityColor returns the appropriate color function for a severity level
func getSeverityColor(severity string) func(format string, a ...interface{}) string {
	switch severity {
	case "critical":
		return color.RedString
	case "high":
		return color.YellowString
	case "medium":
		return color.MagentaString
	case "low":
		return color.BlueString
	default:
		return color.WhiteString
	}
}
