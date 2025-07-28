// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package privileged

import (
	"fmt"

	model "github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// memfdLanguageDetectedFileName MUST BE the same as:
//
//   - #define MEMFD_LANGUAGE_DETECT_FILENAME "dd_language_detected"
//     source: https://github.com/DataDog/auto_inject/blob/13fb55691332a3adeb73ebd9859bc559c493cc57/src/include/dd/memfd.h#L3
const memfdLanguageDetectedFileName = "dd_language_detected"

// memFdMaxSize is the longest string we're going to be
// reading from this memfd file. Because this is _only_
// language, this is likely to be very short (under 10 chars)
// since it's a known set of things we're writing at the moment.
const memFdMaxSize = 10

// NewInjectorDetector is a language Detector that uses
// data written by APM auto_inject to detect a language
// for a process.
func NewInjectorDetector() model.Detector {
	return injectorDetector{
		hostProc: kernel.ProcFSRoot(),
	}
}

type injectorDetector struct {
	hostProc string
}

func (i injectorDetector) DetectLanguage(proc model.Process) (model.Language, error) {
	data, err := kernel.GetProcessMemFdFile(
		int(proc.GetPid()),
		i.hostProc,
		memfdLanguageDetectedFileName,
		memFdMaxSize,
	)
	if err != nil {
		return model.Language{}, err
	}

	var name model.LanguageName
	switch string(data) {
	case "nodejs", "js", "node":
		name = model.Node
	case "php":
		name = model.PHP
	case "jvm", "java":
		name = model.Java
	case "python":
		name = model.Python
	case "ruby":
		name = model.Ruby
	case "dotnet":
		name = model.Dotnet
	default:
		return model.Language{}, fmt.Errorf("unknown language detected %s", data)
	}

	return model.Language{Name: name}, nil
}
