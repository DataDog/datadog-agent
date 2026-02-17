// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package adannotation

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
)

type ADAnnotationIssue struct{}

func NewADAnnotationIssue() *ADAnnotationIssue {
	return &ADAnnotationIssue{}
}

func (A *ADAnnotationIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return &healthplatform.Issue{
		Id:          "misconfigured-ad-annotation",
		IssueName:   "misconfigured_ad_annotation",
		Description: "Description",
		Category:    "autodiscovery",
		Location:    "core",
		Severity:    "medium",
		DetectedAt:  "",
		Extra:       nil,
		Remediation: nil, // TODO: Add remediation steps
		Tags:        nil,
	}, nil
}
