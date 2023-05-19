// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// knownPrefixes maps languages names to their prefix
var knownPrefixes = map[string]LanguageName{
	"python": python,
	"java":   java,
}

func languageNameFromCommandLine(cmdline []string) (LanguageName, error) {
	exe := getExe(cmdline)
	for prefix, language := range knownPrefixes {
		if strings.HasPrefix(exe, prefix) {
			return language, nil
		}
	}
	return unknown, fmt.Errorf("unknown executable: %s", exe)
}

func DetectLanguage(procs []procutil.Process) []*Language {
	langs := make([]*Language, len(procs))
	for i, proc := range procs {
		languageName, err := languageNameFromCommandLine(proc.Cmdline)
		if err == nil {
			log.Trace("detected languageName:", languageName)
		}
		langs[i] = &Language{Name: languageName}
	}
	return langs
}
