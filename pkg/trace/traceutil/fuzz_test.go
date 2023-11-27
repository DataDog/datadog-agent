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

var normalizationSeedCorpus = []string{
	"key:val",
	"Data🐨dog🐶 繋がっ⛰てて",
	"Test Conversion Of Weird !@#$%^&**() Characters",
	"test\x99\x8faaa",
	"A00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000 000000000000",
}

func FuzzNormalizeTag(f *testing.F) {
	normalize := func(in, _ string) (string, error) { return NormalizeTag(in), nil }
	fuzzNormalization(f, normalizationSeedCorpus, maxTagLength, normalize)
}

func FuzzNormalizeTagValue(f *testing.F) {
	normalize := func(in, _ string) (string, error) { return NormalizeTagValue(in), nil }
	fuzzNormalization(f, normalizationSeedCorpus, maxTagLength, normalize)
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

var svcNormalizationSeedCorpus = []string{
	"good",
	"bad$",
	"Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.",
	"127.0.0.1",
	":4500",
	"120.site.platform.com-db.replica1",
}

func FuzzNormalizeService(f *testing.F) {
	fuzzNormalization(f, svcNormalizationSeedCorpus, MaxServiceLen, NormalizeService)
}

func FuzzNormalizePeerService(f *testing.F) {
	normalize := func(in, _ string) (string, error) { return NormalizePeerService(in) }
	fuzzNormalization(f, svcNormalizationSeedCorpus, MaxServiceLen, normalize)
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
