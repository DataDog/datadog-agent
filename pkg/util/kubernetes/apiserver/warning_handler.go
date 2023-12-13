// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var supressedWarning = regexp.MustCompile(`.*is deprecated in v.*`)

// CustomWarningLogger is a custom logger to wrap warning logs
type CustomWarningLogger struct{}

// HandleWarningHeader suppresses some warning logs
func (CustomWarningLogger) HandleWarningHeader(code int, agent string, message string) {
	if code != 299 || len(message) == 0 || supressedWarning.MatchString(message) {
		return
	}

	log.Warn(message)
}
