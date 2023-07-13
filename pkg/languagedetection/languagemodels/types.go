// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagemodels

// LanguageName is a string enum that represents a detected language name.
type LanguageName string

const (
	Node    LanguageName = "node"
	Dotnet  LanguageName = "dotnet"
	Python  LanguageName = "python"
	Java    LanguageName = "java"
	Ruby    LanguageName = "ruby"
	Unknown LanguageName = ""
)

// Language contains metadata collected from the call to `DetectLanguage`
type Language struct {
	Name LanguageName
}
