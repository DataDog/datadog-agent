// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tests

import (
	"github.com/DataDog/datadog-agent/pkg/compliance"
	// Register constant resource so that unit tests don't have to
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/constants"
)

// NewTestRule returns a new basic Rego based to be used by tests
func NewTestRule(resource compliance.RegoInput, kind, module string) *compliance.RegoRule {
	return &compliance.RegoRule{
		RuleCommon: compliance.RuleCommon{
			ID: "rule-id",
		},
		Inputs: []compliance.RegoInput{
			resource,
			{
				ResourceCommon: compliance.ResourceCommon{
					Constants: &compliance.ConstantsResource{
						Values: map[string]interface{}{
							"resource_type": kind,
						},
					},
				},
			},
		},
		Imports: []string{
			"../../rego/rego_helpers/helpers.rego",
		},
		Module: module,
	}
}
