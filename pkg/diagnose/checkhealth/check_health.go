// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package checkhealth

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
)

// DiagnoseCheckHealth returns diagnoses for all checks, focusing on those with warnings or errors
func DiagnoseCheckHealth(collectorComponent collector.Component, log log.Component) []diagnose.Diagnosis {
	var diagnoses []diagnose.Diagnosis

	// Get all running checks
	checks := collectorComponent.GetChecks()
	if len(checks) == 0 {
		diagnoses = append(diagnoses, diagnose.Diagnosis{
			Status:      diagnose.DiagnosisFail,
			Name:        "No checks running",
			Category:    "check-health",
			Diagnosis:   "No checks are currently running. This may indicate a configuration issue.",
			Remediation: "Check your agent configuration and ensure checks are properly configured.",
		})
		return diagnoses
	}

	// Track problematic checks
	var problematicChecks []string
	var checkDetails []string

	// Analyze each check
	for _, ch := range checks {
		checkName := ch.String()
		checkID := string(ch.ID())

		// Get stats for this check
		stats, found := expvars.CheckStats(ch.ID())
		if !found {
			// Check hasn't run yet
			checkDetails = append(checkDetails, fmt.Sprintf("- %s: Not run yet", checkName))
			continue
		}

		// Check for errors
		if stats.LastError != "" {
			problematicChecks = append(problematicChecks, checkName)
			checkDetails = append(checkDetails, fmt.Sprintf("- %s: ERROR - %s", checkName, stats.LastError))

			// Add diagnosis for this error
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:      diagnose.DiagnosisFail,
				Name:        fmt.Sprintf("Check %s has error", checkName),
				Category:    "check-health",
				Diagnosis:   fmt.Sprintf("Check %s (ID: %s) reported error: %s", checkName, checkID, stats.LastError),
				Remediation: fmt.Sprintf("Run 'datadog-agent check %s --log-level error' for detailed diagnostics", checkName),
				RawError:    stats.LastError,
			})
		}

		// Check for warnings
		if len(stats.LastWarnings) > 0 {
			problematicChecks = append(problematicChecks, checkName)
			warnings := strings.Join(stats.LastWarnings, "; ")
			checkDetails = append(checkDetails, fmt.Sprintf("- %s: WARNING - %s", checkName, warnings))

			// Add diagnosis for warnings
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:      diagnose.DiagnosisWarning,
				Name:        fmt.Sprintf("Check %s has warnings", checkName),
				Category:    "check-health",
				Diagnosis:   fmt.Sprintf("Check %s (ID: %s) reported warnings: %s", checkName, checkID, warnings),
				Remediation: fmt.Sprintf("Run 'datadog-agent check %s --log-level error' for detailed diagnostics", checkName),
			})
		}

		// Check for high error rates
		if stats.TotalRuns > 0 && float64(stats.TotalErrors)/float64(stats.TotalRuns) > 0.5 {
			problematicChecks = append(problematicChecks, checkName)
			errorRate := float64(stats.TotalErrors) / float64(stats.TotalRuns) * 100
			checkDetails = append(checkDetails, fmt.Sprintf("- %s: HIGH ERROR RATE - %.1f%% (%d/%d runs)",
				checkName, errorRate, stats.TotalErrors, stats.TotalRuns))

			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:   diagnose.DiagnosisFail,
				Name:     fmt.Sprintf("Check %s has high error rate", checkName),
				Category: "check-health",
				Diagnosis: fmt.Sprintf("Check %s (ID: %s) has %.1f%% error rate (%d errors in %d runs)",
					checkName, checkID, errorRate, stats.TotalErrors, stats.TotalRuns),
				Remediation: fmt.Sprintf("Run 'datadog-agent check %s --log-level error' for detailed diagnostics", checkName),
			})
		}

		// Check for long execution times
		if stats.AverageExecutionTime > 5000 { // More than 5 seconds
			checkDetails = append(checkDetails, fmt.Sprintf("- %s: SLOW - %.2fs average execution time",
				checkName, float64(stats.AverageExecutionTime)/1000))

			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:   diagnose.DiagnosisWarning,
				Name:     fmt.Sprintf("Check %s is slow", checkName),
				Category: "check-health",
				Diagnosis: fmt.Sprintf("Check %s (ID: %s) has slow average execution time: %.2fs",
					checkName, checkID, float64(stats.AverageExecutionTime)/1000),
				Remediation: "Consider optimizing the check or increasing its interval",
			})
		}
	}

	// Check for loader errors
	loaderErrors := pkgcollector.GetLoaderErrors()
	for check, errors := range loaderErrors {
		problematicChecks = append(problematicChecks, check)
		errorDetails := make([]string, 0, len(errors))
		for loader, err := range errors {
			errorDetails = append(errorDetails, fmt.Sprintf("%s: %s", loader, err))
		}
		checkDetails = append(checkDetails, fmt.Sprintf("- %s: LOADER ERROR - %s", check, strings.Join(errorDetails, "; ")))

		diagnoses = append(diagnoses, diagnose.Diagnosis{
			Status:      diagnose.DiagnosisFail,
			Name:        fmt.Sprintf("Check %s failed to load", check),
			Category:    "check-health",
			Diagnosis:   fmt.Sprintf("Check %s failed to load: %s", check, strings.Join(errorDetails, "; ")),
			Remediation: "Check the check configuration and ensure all dependencies are available",
		})
	}

	// Check for autodiscovery config errors
	configErrors := autodiscoveryimpl.GetConfigErrors()
	for check, err := range configErrors {
		problematicChecks = append(problematicChecks, check)
		checkDetails = append(checkDetails, fmt.Sprintf("- %s: CONFIG ERROR - %s", check, err))

		diagnoses = append(diagnoses, diagnose.Diagnosis{
			Status:      diagnose.DiagnosisFail,
			Name:        fmt.Sprintf("Check %s has config error", check),
			Category:    "check-health",
			Diagnosis:   fmt.Sprintf("Check %s has configuration error: %s", check, err),
			Remediation: "Check the autodiscovery configuration for this check",
		})
	}

	// Add summary diagnosis
	if len(problematicChecks) == 0 {
		diagnoses = append(diagnoses, diagnose.Diagnosis{
			Status:    diagnose.DiagnosisSuccess,
			Name:      "All checks healthy",
			Category:  "check-health",
			Diagnosis: fmt.Sprintf("All %d checks are running without errors or warnings", len(checks)),
		})
	} else {
		diagnoses = append(diagnoses, diagnose.Diagnosis{
			Status:      diagnose.DiagnosisFail,
			Name:        "Check health summary",
			Category:    "check-health",
			Diagnosis:   fmt.Sprintf("Found %d problematic checks:\n%s", len(problematicChecks), strings.Join(checkDetails, "\n")),
			Remediation: "Run manual diagnostics on problematic checks using 'datadog-agent check <check_name> --log-level error'",
		})
	}

	// Run manual diagnostics on problematic checks (limited to first 3 to avoid overwhelming output)
	manualDiagnoses := runManualDiagnostics(problematicChecks[:min(len(problematicChecks), 3)], log)
	diagnoses = append(diagnoses, manualDiagnoses...)

	return diagnoses
}

// runManualDiagnostics runs manual check diagnostics on problematic checks
func runManualDiagnostics(problematicChecks []string, log log.Component) []diagnose.Diagnosis {
	var diagnoses []diagnose.Diagnosis

	for _, checkName := range problematicChecks {
		// Run manual check with error logging
		cmd := exec.Command("datadog-agent", "check", checkName, "--log-level", "error")
		output, err := cmd.CombinedOutput()

		if err != nil {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:    diagnose.DiagnosisFail,
				Name:      fmt.Sprintf("Manual check failed for %s", checkName),
				Category:  "check-health",
				Diagnosis: fmt.Sprintf("Failed to run manual check for %s: %v", checkName, err),
				RawError:  err.Error(),
			})
			continue
		}

		outputStr := string(output)

		// Check if the output contains error indicators
		if strings.Contains(outputStr, "ERROR") || strings.Contains(outputStr, "Error") {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:      diagnose.DiagnosisFail,
				Name:        fmt.Sprintf("Manual check errors for %s", checkName),
				Category:    "check-health",
				Diagnosis:   fmt.Sprintf("Manual check for %s revealed errors:\n%s", checkName, outputStr),
				Remediation: "Review the check configuration and logs for this integration",
			})
		} else if strings.Contains(outputStr, "WARN") || strings.Contains(outputStr, "Warn") {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:      diagnose.DiagnosisWarning,
				Name:        fmt.Sprintf("Manual check warnings for %s", checkName),
				Category:    "check-health",
				Diagnosis:   fmt.Sprintf("Manual check for %s revealed warnings:\n%s", checkName, outputStr),
				Remediation: "Review the check configuration for potential issues",
			})
		} else {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Status:    diagnose.DiagnosisSuccess,
				Name:      fmt.Sprintf("Manual check passed for %s", checkName),
				Category:  "check-health",
				Diagnosis: fmt.Sprintf("Manual check for %s completed successfully:\n%s", checkName, outputStr),
			})
		}
	}

	return diagnoses
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
