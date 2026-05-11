// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package store

const (
	// ADMisconfigurationIssueID is the unique identifier for AD misconfiguration issues
	ADMisconfigurationIssueID = "ad-misconfiguration"

	// ADMisconfigurationCheckName is the name of the check for AD misconfigurations
	ADMisconfigurationCheckName = "Autodiscovery Misconfiguration"

	// InvalidConfigIssueID is the stable identifier shared by both the in-Fx
	// schema check and the lite-mode rescue path, so the backend dedupes both
	// detection paths into a single Agent Health issue.
	InvalidConfigIssueID = "invalid-config"

	// InvalidConfigCheckID is the stable ID for the periodic schema-validation check.
	InvalidConfigCheckID = "invalid-config-check"

	// InvalidConfigCheckName is the human-readable name shown in diagnose / flare.
	InvalidConfigCheckName = "Datadog Agent Configuration"
)
