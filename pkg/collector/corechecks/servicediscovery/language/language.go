// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package language provides functionality to detect the programming language for a given process.
package language

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"go.uber.org/zap"
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
	procToLanguage = map[string]Language{
		"java":    Java,
		"node":    Node,
		"nodemon": Node,
		"python":  Python,
		"python3": Python,
		"dotnet":  DotNet,
		"ruby":    Ruby,
		"bundle":  Ruby,
	}
)

// Detect attempts to detect the Language from the provided process information.
func (lf Finder) Detect(args []string, envs []string) (Language, bool) {
	lang := lf.findLang(ProcessInfo{
		Args: args,
		Envs: envs,
	})
	if lang == "" {
		return Unknown, false
	}
	return lang, true
}

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
	Envs []string
}

// FileReader attempts to read the most representative file associated to a process.
func (pi ProcessInfo) FileReader() (io.ReadCloser, bool) {
	fileName := pi.Args[0]
	// if it's an absolute path, use it
	if strings.HasPrefix(fileName, "/") {
		return findFile(fileName)
	}
	for _, env := range pi.Envs {
		if key, val, _ := strings.Cut(env, "="); key == "PATH" {
			paths := strings.Split(val, ":")
			for _, path := range paths {
				if r, found := findFile(path + string(os.PathSeparator) + fileName); found {
					return r, true
				}
			}
		}
	}
	// well, just try it as a relative path, maybe it works
	return findFile(fileName)
}

// Matcher allows to check if a process matches to a concrete language.
type Matcher interface {
	Language() Language
	Match(pi ProcessInfo) bool
}

// New returns a new language Finder.
func New(l *zap.Logger) Finder {
	return Finder{
		Logger: l,
		Matchers: []Matcher{
			PythonScript{},
			RubyScript{},
			DotNetBinary{},
		},
	}
}

// Finder allows to detect the language for a given process.
type Finder struct {
	Logger   *zap.Logger
	Matchers []Matcher
}

func (lf Finder) findLang(pi ProcessInfo) Language {
	lang := findInArgs(pi.Args)
	lf.Logger.Debug("language found", zap.Any("lang", lang))

	// if we can't figure out a language from the command line, try alternate methods
	if lang == "" {
		lf.Logger.Debug("trying alternate methods")
		for _, matcher := range lf.Matchers {
			if matcher.Match(pi) {
				lf.Logger.Debug(fmt.Sprintf("%T matched", matcher))
				lang = matcher.Language()
				break
			}
		}
	}
	return lang
}

func findInArgs(args []string) Language {
	// empty slice passed in
	if len(args) == 0 {
		return ""
	}
	for i := 0; i < len(args); i++ {
		procName := path.Base(args[i])
		// if procName is a known language, return the pos and the language
		if lang, ok := procToLanguage[procName]; ok {
			return lang
		}
	}
	return ""
}
