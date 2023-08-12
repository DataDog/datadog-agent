// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

type languageFromCLI struct {
	name      languagemodels.LanguageName
	validator func(exe string) bool
}

// rubyPattern is a regexp validator for the ruby prefix
var rubyPattern = regexp.MustCompile(`^ruby\d+\.\d+$`)

// knownPrefixes maps languages names to their prefix
var knownPrefixes = map[string]languageFromCLI{
	"python": {name: languagemodels.Python},
	"java": {name: languagemodels.Java, validator: func(exe string) bool {
		if exe == "javac" {
			return false
		}
		return true
	}},
	"ruby": {name: languagemodels.Ruby, validator: func(exe string) bool {
		return rubyPattern.MatchString(exe)
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

	"ruby":  {name: languagemodels.Ruby},
	"rubyw": {name: languagemodels.Ruby},
}

func languageNameFromCommand(command string) languagemodels.LanguageName {
	// First check to see if there is an exact match
	if lang, ok := exactMatches[command]; ok {
		return lang.name
	}

	for prefix, language := range knownPrefixes {
		if strings.HasPrefix(command, prefix) {
			if language.validator != nil {
				isValidResult := language.validator(command)
				if !isValidResult {
					continue
				}
			}
			return language.name
		}
	}

	return languagemodels.Unknown
}

// DetectLanguage uses a combination of commandline parsing and binary analysis to detect a process' language
func DetectLanguage(procs []languagemodels.Process) []*languagemodels.Language {
	langs := make([]*languagemodels.Language, len(procs))
	for i, proc := range procs {
		exe := getExe(proc.GetCmdline())
		languageName := languageNameFromCommand(exe)
		if languageName == languagemodels.Unknown {
			languageName = languageNameFromCommand(proc.GetCommand())
		}
		langs[i] = &languagemodels.Language{Name: languageName}
	}
	return langs
}
