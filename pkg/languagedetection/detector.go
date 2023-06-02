// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"strings"
)

// LanguageName is a string enum that represents a detected language name.
type LanguageName string

const (
	Node    LanguageName = "node"
	Dotnet  LanguageName = "dotnet"
	Python  LanguageName = "python"
	Java    LanguageName = "java"
	Unknown LanguageName = ""
)

// Language contains metadata collected from the call to `DetectLanguage`
type Language struct {
	Name LanguageName
}

type languageFromCLI struct {
	name      LanguageName
	validator func(exe string) bool
}

// knownPrefixes maps languages names to their prefix
var knownPrefixes = map[string]languageFromCLI{
	"python": {name: Python},
	"java": {name: Java, validator: func(exe string) bool {
		if exe == "javac" {
			return false
		}
		return true
	}},
}

// exactMatches maps an exact exe name match to a prefix
var exactMatches = map[string]languageFromCLI{
	"py":     {name: Python},
	"python": {name: Python},

	"java": {name: Java},

	"npm":  {name: Node},
	"node": {name: Node},

	"dotnet": {name: Dotnet},
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

	return Unknown
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
		langs[i] = &Language{Name: languageName}
	}
	return langs
}
