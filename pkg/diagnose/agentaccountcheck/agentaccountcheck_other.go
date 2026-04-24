// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package agentaccountcheck

import (
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

// diagnoseImpl provides a stub implementation for non-Windows platforms
func diagnoseImpl() []diagnose.Diagnosis {
	return []diagnose.Diagnosis{
		{
			Status:      diagnose.DiagnosisSuccess,
			Name:        "Agent Account Check",
			Diagnosis:   "Agent account check is only available on Windows",
			Category:    "agent-account-check",
			Description: "This diagnostic suite is specific to Windows agent user account permissions and group memberships",
		},
	}
}
