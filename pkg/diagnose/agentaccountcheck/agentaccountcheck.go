// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agentaccountcheck provides diagnostic functions for agent user account checks
package agentaccountcheck

import (
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

// Diagnose runs the agent user account check diagnostic suite
func Diagnose() []diagnose.Diagnosis {
	return diagnoseImpl()
}
