// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package store

const (
	// ADMisconfigurationIssueType is the IssueType value for autodiscovery
	// misconfiguration issues, used to look up the template in the issue registry.
	ADMisconfigurationIssueType = "ad-misconfiguration"

	// ADMisconfigurationSource is the Source value reported when the
	// autodiscovery component detects a misconfiguration.
	ADMisconfigurationSource = "autodiscovery"

	// InvalidConfigIssueID is shared across all "invalid-config" detection paths
	InvalidConfigIssueID = "invalid-config"

	// InvalidConfigCheckID matches InvalidConfigIssueID so every detection path
	// produces the same HealthReport.issues map key for de-duplication
	InvalidConfigCheckID = "invalid-config"

	// InvalidConfigCheckName is the human-readable name
	InvalidConfigCheckName = "Datadog Agent Configuration Validation"
)
