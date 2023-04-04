// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build go1.18

package traceutil

import (
	"testing"
	"unicode/utf8"
)

func FuzzNormalizeTag(f *testing.F) {
	seedCorpus := []string{
		"key:val",
		"Data🐨dog🐶 繋がっ⛰てて",
		"Test Conversion Of Weird !@#$%^&**() Characters",
		"test\x99\x8faaa",
		"A00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000 000000000000",
	}
	normalize := func(in, _ string) (string, error) { return NormalizeTag(in), nil }
	fuzzNormalization(f, seedCorpus, maxTagLength, normalize)
}

func FuzzNormalizeName(f *testing.F) {
	seedCorpus := []string{
		"good.one",
		"bad-one",
		"Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.",
	}
	normalize := func(in, _ string) (string, error) { return NormalizeName(in) }
	fuzzNormalization(f, seedCorpus, MaxNameLen, normalize)
}

func FuzzNormalizeService(f *testing.F) {
	seedCorpus := []string{
		"good",
		"bad$",
		"Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.",
	}
	fuzzNormalization(f, seedCorpus, MaxServiceLen, NormalizeService)
}

func fuzzNormalization(f *testing.F, seedCorpus []string, maxLen int, normalize func(in, lang string) (string, error)) {
	for _, sc := range seedCorpus {
		f.Add(sc, "lang")
	}
	f.Fuzz(func(t *testing.T, input, lang string) {
		normalized, err := normalize(input, lang)
		if err != nil {
			t.Skipf("Couldn't normalize input (%s): %v", input, err)
		}
		normalizedTwice, err := normalize(normalized, lang)
		if err != nil {
			t.Fatalf("Normalizing a normalized input returned an error: %v", err)
		}
		if normalizedTwice != normalized {
			t.Fatalf("Normalizing a normalized input didn't return the same input: expected (%s) got (%s)", normalized, normalizedTwice)
		}
		runeCount := utf8.RuneCountInString(normalized)
		if runeCount > maxLen {
			t.Fatalf("Max length (%d) exceeded: runeCount(%s) == %d", maxLen, normalized, runeCount)
		}
	})
}
