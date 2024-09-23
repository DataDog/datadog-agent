// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package language

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// languageNameToLanguageMap translates the constants rom the
// languagedetection package to the constants used in this file. The latter
// are shared with the backend, and at least java/jvm differs in the name
// from the languagedetection package.
var languageNameToLanguageMap = map[languagemodels.LanguageName]Language{
	languagemodels.Go:     Go,
	languagemodels.Node:   Node,
	languagemodels.Dotnet: DotNet,
	languagemodels.Python: Python,
	languagemodels.Java:   Java,
	languagemodels.Ruby:   Ruby,
}

// ProcessInfo holds information about a process.
type ProcessInfo struct {
	Args []string
	Envs map[string]string
}

// FindInArgs tries to detect the language only using the provided command line arguments.
func FindInArgs(exe string, args []string) Language {
	// empty slice passed in
	if len(args) == 0 {
		return ""
	}

	langs := languagedetection.DetectLanguage([]languagemodels.Process{&procutil.Process{
		// Pid doesn't matter since sysprobeConfig is nil
		Pid:     0,
		Cmdline: args,
		Comm:    filepath.Base(exe),
	}}, nil)
	if len(langs) == 0 {
		return ""
	}

	lang := langs[0]
	if lang == nil {
		return ""
	}
	if outLang, ok := languageNameToLanguageMap[lang.Name]; ok {
		return outLang
	}

	return ""
}

// FindUsingPrivilegedDetector tries to detect the language using the provided command line arguments
func FindUsingPrivilegedDetector(detector privileged.LanguageDetector, pid int32) Language {
	langs := detector.DetectWithPrivileges([]languagemodels.Process{&procutil.Process{Pid: pid}})
	if len(langs) == 0 {
		return ""
	}

	lang := langs[0]
	if outLang, ok := languageNameToLanguageMap[lang.Name]; ok {
		return outLang
	}

	return ""
}
