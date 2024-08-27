// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package language provides functionality to detect the programming language for a given process.
package language

import (
	"io"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// Language represents programming languages.
type Language string

const (
	// Unknown is used when the language could not be detected.
	Unknown Language = "UNKNOWN"
	// Java represents JVM languages.
	Java Language = "jvm"
	// Node represents Node.js.
	Node Language = "nodejs"
	// Python represents Python.
	Python Language = "python"
	// Ruby represents Ruby.
	Ruby Language = "ruby"
	// DotNet represents .Net.
	DotNet Language = "dotnet"
	// Go represents Go.
	Go Language = "go"
	// CPlusPlus represents C++.
	CPlusPlus Language = "cpp"
	// PHP represents PHP.
	PHP Language = "php"
)

var (
	// languageNameToLanguageMap translates the constants rom the
	// languagedetection package to the constants used in this file. The latter
	// are shared with the backend, and at least java/jvm differs in the name
	// from the languagedetection package.
	languageNameToLanguageMap = map[languagemodels.LanguageName]Language{
		languagemodels.Go:     Go,
		languagemodels.Node:   Node,
		languagemodels.Dotnet: DotNet,
		languagemodels.Python: Python,
		languagemodels.Java:   Java,
		languagemodels.Ruby:   Ruby,
	}
)

func findFile(fileName string) (io.ReadCloser, bool) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, false
	}
	return f, true
}

// ProcessInfo holds information about a process.
type ProcessInfo struct {
	Args []string
	Envs map[string]string
}

// FileReader attempts to read the most representative file associated to a process.
func (pi ProcessInfo) FileReader() (io.ReadCloser, bool) {
	if len(pi.Args) == 0 {
		return nil, false
	}
	fileName := pi.Args[0]
	// if it's an absolute path, use it
	if strings.HasPrefix(fileName, "/") {
		return findFile(fileName)
	}
	if val, ok := pi.Envs["PATH"]; ok {
		paths := strings.Split(val, ":")
		for _, path := range paths {
			if r, found := findFile(path + string(os.PathSeparator) + fileName); found {
				return r, true
			}
		}

	}

	// well, just try it as a relative path, maybe it works
	return findFile(fileName)
}

// FindInArgs tries to detect the language only using the provided command line arguments.
func FindInArgs(args []string) Language {
	// empty slice passed in
	if len(args) == 0 {
		return ""
	}

	langs := languagedetection.DetectLanguage([]languagemodels.Process{&procutil.Process{
		// Pid doesn't matter since sysprobeConfig is nil
		Pid:     0,
		Cmdline: args,
		Comm:    args[0],
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
