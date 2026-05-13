// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package store

const (
	// ADMisconfigurationIssueName is the snake_case issue name for autodiscovery
	// misconfiguration issues, used as the template registry key and proto IssueName field.
	ADMisconfigurationIssueName = "ad_misconfiguration"

	// ADMisconfigurationSource is the Source value reported when the
	// autodiscovery component detects a misconfiguration.
	ADMisconfigurationSource = "autodiscovery"

	// InvalidConfigIssueID is shared across all "invalid-config" detection paths
	InvalidConfigIssueID = "invalid-config"

	// InvalidConfigCheckID is the identifier for the schema-validation check
	InvalidConfigCheckID = "invalid-config-check"

	// InvalidConfigCheckName is the human-readable name
	InvalidConfigCheckName = "Datadog Agent Configuration Validation"
)
