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

const goCoverDirWarning = "warning: GOCOVERDIR not set, no coverage data emitted"

// suppressGoCoverWarning suppresses the go cover warning when the coverage pipeline is enabled.
func suppressGoCoverWarning(s string) string {
	coveragePipeline, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.CoveragePipeline, false)
	if err != nil || !coveragePipeline {
		return s
	}
	return removeGoCoverWarningLines(s)
}

// removeGoCoverWarningLines removes entire lines containing the coverage warning. The warning can be
// part of a prefixed agent log line, so removing only the warning text would join the prefix to the
// following line and could corrupt structured output.
func removeGoCoverWarningLines(s string) string {
	var output strings.Builder
	for _, line := range strings.SplitAfter(s, "\n") {
		if strings.Contains(line, goCoverDirWarning) {
			continue
		}
		output.WriteString(line)
	}
	return output.String()
}
