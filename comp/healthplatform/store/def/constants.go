// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package store

const (
	// ADMisconfigurationSource is the Source value reported when the
	// autodiscovery component detects a misconfiguration.
	ADMisconfigurationSource = "autodiscovery"

	// ADAnnotationIssueID is the stable IssueID prefix for AD annotation misconfiguration issues.
	// External reporters append a per-entity suffix separated by a colon:
	//   ADAnnotationIssueID + ":" + entityName
	ADAnnotationIssueID = "ad-annotation"

	// ADTemplateIssueID is the stable IssueID prefix for AD template resolution failure issues.
	// External reporters append name, service-id, and digest segments separated by colons:
	//   ADTemplateIssueID + ":" + name + ":" + serviceID + ":" + digest
	ADTemplateIssueID = "ad-template"
)
