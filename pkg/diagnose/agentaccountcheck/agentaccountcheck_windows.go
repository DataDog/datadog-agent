// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package agentaccountcheck

import (
	"fmt"
	"strings"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// diagnoseImpl provides the Windows-specific implementation for agent account check diagnosis
func diagnoseImpl() []diagnose.Diagnosis {
	var diagnoses []diagnose.Diagnosis

	// Check agent account groups
	actualGroups, hasDesiredGroups, err := winutil.DoesAgentUserHaveDesiredGroups()
	diagnoses = append(diagnoses, createGroupsDiagnosis(actualGroups, hasDesiredGroups, err))

	// Check agent account rights
	actualRights, hasDesiredRights, err := winutil.DoesAgentUserHaveDesiredRights()
	diagnoses = append(diagnoses, createRightsDiagnosis(actualRights, hasDesiredRights, err))

	return diagnoses
}

// createGroupsDiagnosis creates a diagnosis for agent account group membership
func createGroupsDiagnosis(actualGroups []string, hasDesired bool, err error) diagnose.Diagnosis {
	name := "Agent Account Group Membership"
	category := "agent-account-check"
	requiredGroups := []string{"Event Log Readers", "Performance Log Users", "Performance Monitor Users"}

	if err != nil {
		if isAccessDenied(err) {
			return diagnose.Diagnosis{
				Status:      diagnose.DiagnosisWarning,
				Name:        name,
				Diagnosis:   fmt.Sprintf("Cannot verify agent account group membership due to insufficient privileges.\n  Expected: %v\n  Detected: unable to check due to access denied", requiredGroups),
				Category:    category,
				RawError:    err.Error(),
				Remediation: "Run as Administrator to check agent account group membership, or ensure the agent service is running with appropriate privileges.",
			}
		}
		if isAgentNotInstalled(err) {
			return diagnose.Diagnosis{
				Status:      diagnose.DiagnosisWarning,
				Name:        name,
				Diagnosis:   fmt.Sprintf("Cannot verify agent account group membership because agent is not installed.\n  Expected: %v\n  Detected: agent not found", requiredGroups),
				Category:    category,
				RawError:    err.Error(),
				Remediation: "Install the Datadog Agent first, or ensure the agent installation completed successfully.",
			}
		}
		return diagnose.Diagnosis{
			Status:      diagnose.DiagnosisUnexpectedError,
			Name:        name,
			Diagnosis:   fmt.Sprintf("Failed to check agent account group membership.\n  Expected: %v\n  Detected: error occurred", requiredGroups),
			Category:    category,
			RawError:    err.Error(),
			Remediation: "Check if the agent is running with appropriate permissions.",
		}
	}

	if hasDesired {
		return diagnose.Diagnosis{
			Status:    diagnose.DiagnosisSuccess,
			Name:      name,
			Diagnosis: "Agent account has all required group memberships",
			Category:  category,
		}
	}

	// Agent account doesn't have all required groups
	return diagnose.Diagnosis{
		Status:      diagnose.DiagnosisFail,
		Name:        name,
		Diagnosis:   fmt.Sprintf("Agent account is missing required group memberships.\n  Expected: %v\n  Detected: %v", requiredGroups, actualGroups),
		Category:    category,
		Remediation: "Add the agent account to the missing groups using Computer Management or run: net localgroup \"<group_name>\" \"<username>\" /add",
	}
}

// createRightsDiagnosis creates a diagnosis for agent account rights
func createRightsDiagnosis(actualRights []string, hasDesired bool, err error) diagnose.Diagnosis {
	name := "Agent Account Rights"
	category := "agent-account-check"
	requiredRights := []string{"SeServiceLogonRight", "SeDenyInteractiveLogonRight", "SeDenyNetworkLogonRight", "SeDenyRemoteInteractiveLogonRight"}

	if err != nil {
		if isAccessDenied(err) {
			return diagnose.Diagnosis{
				Status:      diagnose.DiagnosisWarning,
				Name:        name,
				Diagnosis:   fmt.Sprintf("Cannot verify agent account rights due to insufficient privileges.\n  Expected: %v\n  Detected: unable to check due to access denied", requiredRights),
				Category:    category,
				RawError:    err.Error(),
				Remediation: "Run as Administrator to check agent account rights, or ensure the agent service is running with appropriate privileges.",
			}
		}
		if isAgentNotInstalled(err) {
			return diagnose.Diagnosis{
				Status:      diagnose.DiagnosisWarning,
				Name:        name,
				Diagnosis:   fmt.Sprintf("Cannot verify agent account rights because agent is not installed.\n  Expected: %v\n  Detected: agent not found", requiredRights),
				Category:    category,
				RawError:    err.Error(),
				Remediation: "Install the Datadog Agent first, or ensure the agent installation completed successfully.",
			}
		}
		return diagnose.Diagnosis{
			Status:      diagnose.DiagnosisUnexpectedError,
			Name:        name,
			Diagnosis:   fmt.Sprintf("Failed to check agent account rights.\n  Expected: %v\n  Detected: error occurred", requiredRights),
			Category:    category,
			RawError:    err.Error(),
			Remediation: "Check if the agent is running with appropriate permissions.",
		}
	}

	if hasDesired {
		return diagnose.Diagnosis{
			Status:    diagnose.DiagnosisSuccess,
			Name:      name,
			Diagnosis: "Agent account has all required account rights",
			Category:  category,
		}
	}

	// Agent account doesn't have all required rights
	return diagnose.Diagnosis{
		Status:      diagnose.DiagnosisFail,
		Name:        name,
		Diagnosis:   fmt.Sprintf("Agent account is missing required account rights.\n  Expected: %v\n  Detected: %v", requiredRights, actualRights),
		Category:    category,
		Remediation: "Grant the missing rights using Local Security Policy (secpol.msc) or run the agent installer as Administrator.",
	}
}

// isAccessDenied checks if the error is due to insufficient privileges
func isAccessDenied(err error) bool {
	errMsg := err.Error()
	return strings.Contains(errMsg, "access denied") ||
		strings.Contains(errMsg, "administrator privileges may be required")
}

// isAgentNotInstalled checks if the error is due to missing agent installation
func isAgentNotInstalled(err error) bool {
	errMsg := err.Error()
	return strings.Contains(errMsg, "could not open registry key") ||
		strings.Contains(errMsg, "cannot find the file specified")
}
