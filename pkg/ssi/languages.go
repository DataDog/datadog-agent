// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ssi

import "slices"

// SupportedLanguages contains the languages supported by SSI library injection.
var SupportedLanguages = []string{
	"java",
	"js",
	"python",
	"dotnet",
	"ruby",
	"php", // PHP only works with injection v2; no environment variables are set in any case.
	"c",
}

// IsLanguageSupported reports whether SSI supports library injection for the language.
func IsLanguageSupported(lang string) bool {
	return slices.Contains(SupportedLanguages, lang)
}
