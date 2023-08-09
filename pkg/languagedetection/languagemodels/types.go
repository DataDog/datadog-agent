// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package languagemodels TODO comment
package languagemodels

// LanguageName is a string enum that represents a detected language name.
type LanguageName string

// This const block should have a comment or be unexported
const (
	Node    LanguageName = "node"
	Dotnet  LanguageName = "dotnet"
	Python  LanguageName = "python"
	Java    LanguageName = "java"
	Unknown LanguageName = ""
)

// Language contains metadata collected from the call to `DetectLanguage`
type Language struct {
	Name LanguageName
}
