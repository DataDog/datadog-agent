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
)
