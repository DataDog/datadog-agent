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

	// InvalidConfigIssueID is the unique identifier for an unparseable or
	// schema-invalid datadog.yaml. The same identifier is used by lite-mode
	// rescue (pkg/config/lite/rescue.go) so the backend dedupes both
	// detection paths into a single Agent Health issue.
	InvalidConfigIssueID = "invalid-config"

	// InvalidConfigCheckID is the stable check ID for the periodic in-Fx
	// schema-validation check.
	InvalidConfigCheckID = "invalid-config-check"

	// InvalidConfigCheckName is the human-readable name shown in diagnose
	// and flare output for the periodic schema-validation check.
	InvalidConfigCheckName = "Datadog Agent Configuration"
)
