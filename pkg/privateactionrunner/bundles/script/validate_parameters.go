// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_script

import (
	workflowjsonschema "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/workflowjsonschema"
)

func validateParameters(params interface{}, parameterSchema map[string]interface{}) error {
	return workflowjsonschema.ValidateParameters(parameterSchema, params)
}
