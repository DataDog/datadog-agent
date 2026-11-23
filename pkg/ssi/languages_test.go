// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ssi

import "testing"

func TestIsLanguageSupported(t *testing.T) {
	supportedLangs := []string{"java", "js", "python", "dotnet", "ruby", "php", "c"}
	for _, lang := range supportedLangs {
		if !IsLanguageSupported(lang) {
			t.Errorf("expected %s to be supported", lang)
		}
	}

	unsupportedLangs := []string{"cobol", "fortran", "go", "rust", ""}
	for _, lang := range unsupportedLangs {
		if IsLanguageSupported(lang) {
			t.Errorf("expected %s to be unsupported", lang)
		}
	}
}
