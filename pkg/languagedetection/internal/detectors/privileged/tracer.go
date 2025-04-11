// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package privileged

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	model "github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// NewTracerDetector is a language Detector that uses
// data written by the tracer to detect a language
// for a process.
func NewTracerDetector() model.Detector {
	return tracerDetector{
		hostProc: kernel.ProcFSRoot(),
	}
}

type tracerDetector struct {
	hostProc string
}

func (i tracerDetector) DetectLanguage(proc model.Process) (model.Language, error) {
	trMeta, err := tracermetadata.GetTracerMetadata(int(proc.GetPid()), i.hostProc)
	if err != nil {
		return model.Language{}, err
	}

	var name model.LanguageName
	switch trMeta.TracerLanguage {
	case "cpp":
		name = model.CPP
	case "python":
		name = model.Python
	case "go":
		name = model.Go
	case "dotnet":
		name = model.Dotnet
	case "php":
		name = model.PHP
	case "nodejs":
		name = model.Node
	case "ruby":
		name = model.Ruby
	case "jvm":
		name = model.Java
	default:
		return model.Language{}, fmt.Errorf("unknown language detected %s", trMeta.TracerLanguage)
	}

	return model.Language{Name: name}, nil
}
