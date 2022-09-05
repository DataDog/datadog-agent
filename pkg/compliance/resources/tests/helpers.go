// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tests

import (
	"github.com/DataDog/datadog-agent/pkg/compliance"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/constants"
)

func NewTestRule(resource compliance.RegoInput, kind, module string) *compliance.RegoRule {
	return &compliance.RegoRule{
		RuleCommon: compliance.RuleCommon{
			ID: "rule-id",
		},
		Inputs: []compliance.RegoInput{
			resource,
			{
				Type: "object",
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
