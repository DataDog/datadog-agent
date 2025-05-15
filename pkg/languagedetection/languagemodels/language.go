// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagemodels

// LanguageName is a string enum that represents a detected language name.
type LanguageName string

const (
	// Go language name.
	Go LanguageName = "go"

	// Node language name.
	Node LanguageName = "node"

	// Dotnet language name.
	Dotnet LanguageName = "dotnet"

	// Python language name.
	Python LanguageName = "python"

	// Java language name.
	Java LanguageName = "java"

	// Ruby language name.
	Ruby LanguageName = "ruby"

	// PHP language name.
	PHP LanguageName = "php"

	// CPP language name.
	CPP LanguageName = "cpp"

	// Unknown language name.
	Unknown LanguageName = ""
)

// Language contains metadata collected from the call to `DetectLanguage`
type Language struct {
	Name    LanguageName
	Version string
}
