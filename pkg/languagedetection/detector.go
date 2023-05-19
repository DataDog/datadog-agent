// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

type LanguageName string

var (
	python  LanguageName = "python"
	java    LanguageName = "java"
	unknown LanguageName = ""
)

type Language struct {
	Name LanguageName
}

type languageFromCLI struct {
	name      LanguageName
	validator func(exe string) bool
}

// knownPrefixes maps languages names to their prefix
var knownPrefixes = map[string]languageFromCLI{
	"python": {name: python},
	"java": {name: java, validator: func(exe string) bool {
		if exe == "javac" || exe == "javac.exe" {
			return false
		}
		return true
	}},
}

// exactMatches maps an exact exe name match to a prefix
var exactMatches = map[string]languageFromCLI{
	"py": {name: python},
}

func languageNameFromCommandLine(cmdline []string) LanguageName {
	exe := getExe(cmdline)

	// First check to see if there is an exact match
	if lang, ok := exactMatches[exe]; ok {
		return lang.name
	}

	for prefix, language := range knownPrefixes {
		if strings.HasPrefix(exe, prefix) {
			if language.validator != nil {
				isValidResult := language.validator(exe)
				if !isValidResult {
					continue
				}
			}
			return language.name
		}
	}

	return unknown
}

type LanguageDetector struct {
	sysprobeutil       net.SysProbeUtil
	sysprobeConfigured bool
}

func NewLanguageDetector(sysprobeConfig config.ConfigReader) (*LanguageDetector, error) {
	var err error
	detector := &LanguageDetector{sysprobeConfigured: true}
	detector.sysprobeutil, err = net.GetRemoteSystemProbeUtil(sysprobeConfig.GetString("system_probe_config.sysprobe_socket"))
	if err != nil {
		_ = log.Warn("Sysprobe is not available, language detection will not use privileged mode")
		detector.sysprobeConfigured = false
	}

	return detector, nil
}

// DetectLanguage uses a combination of commandline parsing and binary analysis to detect a process' language
func (l *LanguageDetector) DetectLanguage(procs []procutil.Process) []*Language {
	langs := make([]*Language, len(procs))
	for i, proc := range procs {
		languageName := languageNameFromCommandLine(proc.Cmdline)

		// If the sysprobe is configured, and we don't know the language, fall back to that environment.
		if languageName == unknown && l.sysprobeConfigured {
			l.sysprobeutil.GetStats()
		}
		langs[i] = &Language{Name: languageName}
	}
	return langs
}

func DetectLanguageWithPrivileges() {

}
