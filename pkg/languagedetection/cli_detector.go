// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// DetectLanguage uses a combination of commandline parsing and binary analysis to detect a process' language
func DetectLanguage(procs []*procutil.Process, sysprobeConfig config.ConfigReader) []*languagemodels.Language {
	langs := make([]*languagemodels.Language, len(procs))
	unknownPids := make([]int32, 0, len(procs))
	langsToModify := make(map[int32]*languagemodels.Language, len(procs))
	for i, proc := range procs {
		lang := &languagemodels.Language{Name: languageNameFromCommandLine(proc.Cmdline)}
		langs[i] = lang
		if lang.Name == languagemodels.Unknown {
			unknownPids = append(unknownPids, proc.Pid)
			langsToModify[proc.Pid] = lang
		}
	}

	if sysprobeConfig != nil && sysprobeConfig.GetBool("system_probe_config.language_detection.enabled") {
		util, err := net.GetRemoteSystemProbeUtil(
			sysprobeConfig.GetString("system_probe_config.sysprobe_socket"),
		)
		if err != nil {
			log.Warn("Failed to request language:", err)
			return langs
		}

		privilegedLangs, err := util.DetectLanguage(unknownPids)
		if err != nil {
			log.Warn("Failed to request language:", err)
			return langs
		}

		for i, pid := range unknownPids {
			*langsToModify[pid] = privilegedLangs[i]
		}
	}
	return langs
}
