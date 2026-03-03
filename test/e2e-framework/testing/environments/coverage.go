// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package environments contains the definitions of the different environments that can be used in a test.
package environments

import "fmt"

// CoverageTargetSpec defines the name of the agent, the command to run to generate the coverage and if the coverage is required
type CoverageTargetSpec struct {
	AgentName       string
	CoverageCommand []string
	Required        bool
}

func updateErrorOutput(target CoverageTargetSpec, outStr []string, errs []error, errorMessage string) ([]string, []error) {
	outStr = append(outStr, fmt.Sprintf("[WARN] %s: %s", target.AgentName, errorMessage))
	if target.Required {
		errs = append(errs, fmt.Errorf("[ERROR] %s: %s", target.AgentName, errorMessage))
	}
	return outStr, errs
}
