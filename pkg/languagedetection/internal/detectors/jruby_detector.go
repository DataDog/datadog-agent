// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package detectors

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

const jrubyClassName = "org.jruby.Main"

// JRubyDetector is a languagedetection.Detector that detects JRuby processes
type JRubyDetector struct{}

//nolint:revive // TODO(PROC) Fix revive linter
func (d JRubyDetector) DetectLanguage(process languagemodels.Process) (languagemodels.Language, error) {
	if process.GetCommand() != "java" {
		return languagemodels.Language{
			Name: languagemodels.Unknown,
		}, nil
	}

	for _, arg := range process.GetCmdline() {
		if strings.TrimSpace(arg) == jrubyClassName {
			return languagemodels.Language{
				Name: languagemodels.Ruby,
			}, nil
		}
	}

	return languagemodels.Language{
		Name: languagemodels.Unknown,
	}, nil
}
