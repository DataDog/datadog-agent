// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package languagemodels

// Detector is an interface for detecting the language of a process
type Detector interface {
	DetectLanguage(proc Process) (Language, error)
}

// Process is an interface that exposes the fields necessary to detect a language
type Process interface {
	GetPid() int32
	GetCommand() string
	GetCmdline() []string
}
