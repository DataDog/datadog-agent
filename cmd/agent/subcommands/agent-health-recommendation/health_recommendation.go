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
			_ = err // explicitly ignore the error to avoid empty block
		}
	}()

	// Run health checks to collect issues
	report, err := healthPlatform.Run(ctx)
	if err != nil {
		return fmt.Errorf("failed to run health checks: %w", err)
	}

	// Display the health report
	if cliParams.jsonOutput {
		// Output as JSON
		jsonData, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal health report: %w", err)
		}
		fmt.Println(string(jsonData))
	} else {
		// Output as formatted text
		displayHealthReport(report)
	}

	return nil
}

// displayHealthReport displays the health report in a user-friendly format
func displayHealthReport(report *healthplatform.HealthReport) {
	fmt.Fprintf(color.Output, "%s Health Check Results\n", color.GreenString("✅"))
	fmt.Fprintf(color.Output, "Host: %s (Agent Version: %s)\n", report.Host.Hostname, report.Host.AgentVersion)
	fmt.Fprintf(color.Output, "Issues Found: %d\n\n", len(report.Issues))

	if len(report.Issues) == 0 {
		fmt.Fprintf(color.Output, "%s All health checks passed!\n", color.GreenString("🎉"))
		return
	}

	fmt.Fprintf(color.Output, "%s Issues:\n", color.YellowString("⚠️"))
	for i, issue := range report.Issues {
		severityColor := getSeverityColor(issue.Severity)
		fmt.Fprintf(color.Output, "\n%d. %s (%s)\n", i+1, issue.Title, severityColor(issue.Severity))
		fmt.Fprintf(color.Output, "   Issue ID: %s\n", issue.ID)
		fmt.Fprintf(color.Output, "   Category: %s\n", issue.Category)
		fmt.Fprintf(color.Output, "   Location: %s\n", issue.Location)
		fmt.Fprintf(color.Output, "   Description: %s\n", issue.Description)

		if issue.DetectedAt != "" {
			fmt.Fprintf(color.Output, "   Detected At: %s\n", issue.DetectedAt)
		}

		if issue.Integration != nil {
			fmt.Fprintf(color.Output, "   Integration: %s\n", *issue.Integration)
		}

		if issue.Extra != "" {
			fmt.Fprintf(color.Output, "   Additional Info: %s\n", issue.Extra)
		}

		if issue.Remediation != nil {
			fmt.Fprintf(color.Output, "   Remediation: %s\n", issue.Remediation.Summary)
			if len(issue.Remediation.Steps) > 0 {
				fmt.Fprintf(color.Output, "   Steps:\n")
				for _, step := range issue.Remediation.Steps {
					fmt.Fprintf(color.Output, "     %d. %s\n", step.Order, step.Text)
				}
			}
			if issue.Remediation.Script != nil {
				fmt.Fprintf(color.Output, "   Script: %s (%s)\n", issue.Remediation.Script.Filename, issue.Remediation.Script.Language)
			}
		}

		if len(issue.Tags) > 0 {
			fmt.Fprintf(color.Output, "   Tags: %s\n", strings.Join(issue.Tags, ", "))
		}
	}
}

// getSeverityColor returns the appropriate color function for a severity level
func getSeverityColor(severity string) func(string, ...interface{}) string {
	switch severity {
	case "critical":
		return color.RedString
	case "high":
		return color.MagentaString
	case "medium":
		return color.YellowString
	case "low":
		return color.CyanString
	default:
		return color.WhiteString
	}
}
