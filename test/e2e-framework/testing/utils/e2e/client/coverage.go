// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// suppressGoCoverWarning suppresses the go cover warning when the coverage pipeline is enabled
func suppressGoCoverWarning(s string) string {
	coveragePipeline, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.CoveragePipeline, false)
	if err != nil {
		return s
	}
	if coveragePipeline {
		return strings.ReplaceAll(s, "warning: GOCOVERDIR not set, no coverage data emitted\n", "")
	}
	return s
}
