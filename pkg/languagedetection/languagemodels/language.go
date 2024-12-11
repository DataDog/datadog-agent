// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagemodels

// LanguageName is a string enum that represents a detected language name.
type LanguageName string

const (
	//nolint:revive // TODO(PROC) Fix revive linter
	Go LanguageName = "go"
	//nolint:revive // TODO(PROC) Fix revive linter
	Node LanguageName = "node"
	//nolint:revive // TODO(PROC) Fix revive linter
	Dotnet LanguageName = "dotnet"
	//nolint:revive // TODO(PROC) Fix revive linter
	Python LanguageName = "python"
	//nolint:revive // TODO(PROC) Fix revive linter
	Java LanguageName = "java"
	//nolint:revive // TODO(PROC) Fix revive linter
	Ruby LanguageName = "ruby"
	//nolint:revive // TODO(PROC) Fix revive linter
	Unknown LanguageName = ""
)

// Language contains metadata collected from the call to `DetectLanguage`
type Language struct {
	Name    LanguageName
	Version string
}
