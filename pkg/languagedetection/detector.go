// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package languagedetection determines the language that a process is written or compiled in.
package languagedetection

import (
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/internal/detectors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var cliDetectors = []languagemodels.Detector{
	detectors.JRubyDetector{},
}

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
		return exe != "javac"
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

// languageNameFromCmdline returns a process's language from its command.
// If the language is not detected, languagemodels.Unknown is returned.
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

const subsystem = "language_detection"

var (
	detectLanguageRuntimeMs = telemetry.NewHistogram(subsystem, "detect_language_ms", nil,
		"The amount of time it took for the call to DetectLanguage to complete.", nil)
	systemProbeLanguageDetectionMs = telemetry.NewHistogram(subsystem, "system_probe_rpc_ms", nil,
		"The amount of time it took for the process agent to message the system probe.", nil)
)

// DetectLanguage uses a combination of commandline parsing and binary analysis to detect a process' language
func DetectLanguage(procs []languagemodels.Process, sysprobeConfig config.ConfigReader) []*languagemodels.Language {
	detectLanguageStart := time.Now()
	defer func() {
		detectLanguageRuntimeMs.Observe(float64(time.Since(detectLanguageStart).Milliseconds()))
	}()

	langs := make([]*languagemodels.Language, len(procs))
	unknownPids := make([]int32, 0, len(procs))
	langsToModify := make(map[int32]*languagemodels.Language, len(procs))
	for i, proc := range procs {
		// Language-specific detectors should precede matches on the command/exe
		for _, detector := range cliDetectors {
			lang, err := detector.DetectLanguage(proc)
			if err != nil {
				log.Warnf("unable to detect language for process %d: %s", proc.GetPid(), err)
				continue
			}

			if lang.Name != languagemodels.Unknown {
				langs[i] = &lang
				break
			}
		}

		if langs[i] != nil {
			continue
		}

		exe := getExe(proc.GetCmdline())
		languageName := languageNameFromCommand(exe)
		if languageName == languagemodels.Unknown {
			languageName = languageNameFromCommand(proc.GetCommand())
		}
		lang := &languagemodels.Language{Name: languageName}
		langs[i] = lang
		if lang.Name == languagemodels.Unknown {
			unknownPids = append(unknownPids, proc.GetPid())
			langsToModify[proc.GetPid()] = lang
		}
	}

	if privilegedLanguageDetectionEnabled(sysprobeConfig) {
		rpcStart := time.Now()
		defer func() {
			systemProbeLanguageDetectionMs.Observe(float64(time.Since(rpcStart).Milliseconds()))
		}()

		log.Trace("[language detection] Requesting language from system probe")
		util, err := net.GetRemoteSystemProbeUtil(
			sysprobeConfig.GetString("system_probe_config.sysprobe_socket"),
		)
		if err != nil {
			log.Warn("[language detection] Failed to request language:", err)
			return langs
		}

		privilegedLangs, err := util.DetectLanguage(unknownPids)
		if err != nil {
			log.Warn("[language detection] Failed to request language:", err)
			return langs
		}

		for i, pid := range unknownPids {
			*langsToModify[pid] = privilegedLangs[i]
		}
	}
	return langs
}

func privilegedLanguageDetectionEnabled(sysProbeConfig config.ConfigReader) bool {
	if sysProbeConfig == nil {
		return false
	}

	// System probe language detection only works on linux operating systems for the moment.
	if runtime.GOOS != "linux" {
		return false
	}

	return sysProbeConfig.GetBool("system_probe_config.language_detection.enabled")
}
