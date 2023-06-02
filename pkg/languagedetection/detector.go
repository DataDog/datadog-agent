// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"strings"
)

type languageFromCLI struct {
	name      languagemodels.LanguageName
	validator func(exe string) bool
}

// knownPrefixes maps languages names to their prefix
var knownPrefixes = map[string]languageFromCLI{
	"python": {name: languagemodels.Python},
	"java": {name: languagemodels.Java, validator: func(exe string) bool {
		if exe == "javac" {
			return false
		}
		return true
	}},
}

// exactMatches maps an exact exe name match to a prefix
var exactMatches = map[string]languageFromCLI{
	"py":     {name: languagemodels.Python},
	"python": {name: languagemodels.Python},

	"java": {name: languagemodels.Java},

	"npm":  {name: languagemodels.Node},
	"node": {name: languagemodels.Node},

	"dotnet": {name: languagemodels.Dotnet},
}

func languageNameFromCommandLine(cmdline []string) languagemodels.LanguageName {
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

	return languagemodels.Unknown
}

// Process is an internal representation of a process struct.
// It is used to prevent dependency loops.
type Process struct {
	Cmdline []string
	Pid     int32
}

// DetectLanguage uses a combination of commandline parsing and binary analysis to detect a process' language
func DetectLanguage(procs []*Process) []*Language {
	langs := make([]*Language, len(procs))
	for i, proc := range procs {
		languageName := languageNameFromCommandLine(proc.Cmdline)
		langs[i] = &languagemodels.Language{Name: languageName}
	}
	return langs
}
