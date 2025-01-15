// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagemodels

// LanguageName is a string enum that represents a detected language name.
type LanguageName string

const (
	// Go is the name of the go language.
	Go LanguageName = "go"

	// Node is the name of the node language.
	Node LanguageName = "node"

	// Dotnet is the name of the dotnet language.
	Dotnet LanguageName = "dotnet"

	// Python is the name of the python language.
	Python LanguageName = "python"

	// Java is the name of the java language.
	Java LanguageName = "java"

	// Ruby is the name of the ruby language.
	Ruby LanguageName = "ruby"

	// PHP is the name of the php language.
	PHP LanguageName = "php"

	// Unknown is when we don't have a language.
	Unknown LanguageName = ""
)

// Language contains metadata collected from the call to `DetectLanguage`
type Language struct {
	Name    LanguageName
	Version string
}
