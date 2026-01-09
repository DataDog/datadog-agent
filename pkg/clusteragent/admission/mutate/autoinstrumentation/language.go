// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
	"strings"
)

// Language is a type to represent an injectable language with an SDK. A language with a version is a Library.
type Language string

const (
	// Java is a constant for the Java language.
	Java Language = "java"
	// Javascript is a constant for the Javascript language.
	Javascript Language = "js"
	// Python is a constant for the Python language.
	Python Language = "python"
	// Dotnet is a constant for the Dotnet language.
	Dotnet Language = "dotnet"
	// Ruby is a constant for the Ruby language.
	Ruby Language = "ruby"
	// PHP is a constant for the PHP language.
	PHP Language = "php"
)

const (
	// JavaDefaultVersion is the default library version for the Java language.
	JavaDefaultVersion = "v1"
	// JavascriptDefaultVersion is the default library version for the Javascript language.
	JavascriptDefaultVersion = "v5"
	// PythonDefaultVersion is the default library version for the Python language.
	PythonDefaultVersion = "v4"
	// DotnetDefaultVersion is the default library version for the Dotnet language.
	DotnetDefaultVersion = "v3"
	// RubyDefaultVersion is the default library version for the Ruby language.
	RubyDefaultVersion = "v2"
	// PHPDefaultVersion is the default library version for the PHP language.
	PHPDefaultVersion = "v1"
)

// DefaultVersionMagicString is a magic string that indicates that the user wishes to utilize the default version for
// the configured language. This is not an image tag, and needs to be resolved in the DefaultVersions map.
const DefaultVersionMagicString = "default"

// SupportedLanguages defines a list of the languages that we support for Single Step Instrumentation.
var SupportedLanguages = []Language{
	Java,
	Javascript,
	Python,
	Dotnet,
	Ruby,
	PHP,
}

// SupportedLanguagesMap defines a map of supported languages for
var SupportedLanguagesMap = map[Language]bool{
	Java:       true,
	Javascript: true,
	Python:     true,
	Dotnet:     true,
	Ruby:       true,
	PHP:        true,
}

// DefaultVersions defines the major library versions we consider "default" for each supported language.
// If not set, we will default to "latest", see defaultLibVersion. If this language does not appear in
// SupportedLanguages, it will not be injected.
var DefaultVersions = map[Language]string{
	Java:       JavaDefaultVersion,
	Dotnet:     DotnetDefaultVersion,
	Python:     PythonDefaultVersion,
	Ruby:       RubyDefaultVersion,
	Javascript: JavascriptDefaultVersion,
	PHP:        PHPDefaultVersion,
}

// NewLanguage validates and converts a string to a language type.
func NewLanguage(lang string) (Language, error) {
	l := Language(lang)
	if !l.IsSupported() {
		return "", fmt.Errorf("language is not supported: %s", lang)
	}
	return l, nil
}

// IsSupported is a helper method to check the supported languages map given a language. TODO(mspicer): we shouldnt need this.
func (l Language) IsSupported() bool {
	_, ok := SupportedLanguagesMap[l]
	return ok
}

// ExtractLibraryLanguage extracts a language given a library name as input. Ex dd-lib-java-init -> Java.
func ExtractLibraryLanguage(lib string) (Language, error) {
	const prefix = "dd-lib-"
	const suffix = "-init"

	if !strings.HasPrefix(lib, prefix) || !strings.HasSuffix(lib, suffix) {
		return "", fmt.Errorf("input does not match format dd-lib-<language>-init")
	}

	lang := Language(strings.TrimSuffix(strings.TrimPrefix(lib, prefix), suffix))
	_, ok := SupportedLanguagesMap[lang]
	if !ok {
		return "", fmt.Errorf("language parsed is not supported: %s", lang)
	}

	return lang, nil
}
