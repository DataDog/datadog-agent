// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"bytes"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeTag(t *testing.T) {
	for _, tt := range []struct{ in, out string }{
		{in: "#test_starting_hash", out: "test_starting_hash"},
		{in: "TestCAPSandSuch", out: "testcapsandsuch"},
		{in: "Test Conversion Of Weird !@#$%^&**() Characters", out: "test_conversion_of_weird_characters"},
		{in: "$#weird_starting", out: "weird_starting"},
		{in: "allowed:c0l0ns", out: "allowed:c0l0ns"},
		{in: "1love", out: "love"},
		{in: "ünicöde", out: "ünicöde"},
		{in: "ünicöde:metäl", out: "ünicöde:metäl"},
		{in: "Data🐨dog🐶 繋がっ⛰てて", out: "data_dog_繋がっ_てて"},
		{in: " spaces   ", out: "spaces"},
		{in: " #hashtag!@#spaces #__<>#  ", out: "hashtag_spaces"},
		{in: ":testing", out: ":testing"},
		{in: "_foo", out: "foo"},
		{in: ":::test", out: ":::test"},
		{in: "contiguous_____underscores", out: "contiguous_underscores"},
		{in: "foo_", out: "foo"},
		{in: "\u017Fodd_\u017Fcase\u017F", out: "\u017Fodd_\u017Fcase\u017F"}, // edge-case
		{in: "", out: ""},
		{in: " ", out: ""},
		{in: "ok", out: "ok"},
		{in: "™Ö™Ö™™Ö™", out: "ö_ö_ö"},
		{in: "AlsO:ök", out: "also:ök"},
		{in: ":still_ok", out: ":still_ok"},
		{in: "___trim", out: "trim"},
		{in: "12.:trim@", out: ":trim"},
		{in: "12.:trim@@", out: ":trim"},
		{in: "fun:ky__tag/1", out: "fun:ky_tag/1"},
		{in: "fun:ky@tag/2", out: "fun:ky_tag/2"},
		{in: "fun:ky@@@tag/3", out: "fun:ky_tag/3"},
		{in: "tag:1/2.3", out: "tag:1/2.3"},
		{in: "---fun:k####y_ta@#g/1_@@#", out: "fun:k_y_ta_g/1"},
		{in: "AlsO:œ#@ö))œk", out: "also:œ_ö_œk"},
		{in: "test\x99\x8faaa", out: "test_aaa"},
		{in: "test\x99\x8f", out: "test"},
		{in: strings.Repeat("a", 888), out: strings.Repeat("a", 200)},
		{
			in: func() string {
				b := bytes.NewBufferString("a")
				for i := 0; i < 799; i++ {
					_, err := b.WriteRune('🐶')
					assert.NoError(t, err)
				}
				_, err := b.WriteRune('b')
				assert.NoError(t, err)
				return b.String()
			}(),
			out: "a", // 'b' should have been truncated
		},
		{"a" + string(unicode.ReplacementChar), "a"},
		{"a" + string(unicode.ReplacementChar) + string(unicode.ReplacementChar), "a"},
		{"a" + string(unicode.ReplacementChar) + string(unicode.ReplacementChar) + "b", "a_b"},
		{
			in:  "A00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000 000000000000",
			out: "a00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000_0",
		},
	} {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, tt.out, NormalizeTag(tt.in), tt.in)
		})
	}
}

func benchNormalize(tag string, normFn func(string) string) func(b *testing.B) {
	return func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			normFn(tag)
		}
	}
}

type benchCase struct {
	caseName string
	caseVal  string
}

var benchCases = []benchCase{
	{caseName: "ok", caseVal: "good_tag"},
	{caseName: "trim", caseVal: "___trim_left"},
	{caseName: "trim-both", caseVal: "___trim_right@@#!"},
	{caseName: "plenty", caseVal: "fun:ky_ta@#g/1"},
	{caseName: "more", caseVal: "fun:k####y_ta@#g/1_@@#"},
}

func BenchmarkNormalizeTag(b *testing.B) {
	for _, c := range benchCases {
		b.Run(c.caseName, benchNormalize(c.caseVal, NormalizeTag))
	}
}

func BenchmarkNormalizeTagValue(b *testing.B) {
	cases := append(benchCases, benchCase{caseName: "digit", caseVal: "1service-name"})
	for _, c := range cases {
		b.Run(c.caseName, benchNormalize(c.caseVal, NormalizeTagValue))
	}
}

func TestNormalizeName(t *testing.T) {
	testCases := []struct {
		name       string
		normalized string
		err        error
	}{
		{
			name:       "",
			normalized: DefaultSpanName,
			err:        ErrEmpty,
		},
		{
			name:       "good",
			normalized: "good",
			err:        nil,
		},
		{
			name:       "Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.Too-Long-.",
			normalized: "Too_Long.Too_Long.Too_Long.Too_Long.Too_Long.Too_Long.Too_Long.Too_Long.Too_Long.Too_Long.",
			err:        ErrTooLong,
		},
		{
			name:       "bad-name",
			normalized: "bad_name",
			err:        nil,
		},
	}
	for _, testCase := range testCases {
		out, err := NormalizeName(testCase.name)
		assert.Equal(t, testCase.normalized, out)
		assert.Equal(t, testCase.err, err)
	}
}

type serviceNormalizationCase struct {
	service    string
	normalized string
	err        error
}

var svcNormCases = []serviceNormalizationCase{
	{
		service:    "good",
		normalized: "good",
		err:        nil,
	},
	{
		service:    "127.0.0.1",
		normalized: "127.0.0.1",
		err:        nil,
	},
	{
		service:    "127.site.platform-db-replica1",
		normalized: "127.site.platform-db-replica1",
		err:        nil,
	},
	{
		service:    "hyphenated-service-name",
		normalized: "hyphenated-service-name",
		err:        nil,
	},
	{
		service:    "🐨animal-db🐶",
		normalized: "_animal-db",
		err:        nil,
	},
	{
		service:    "🐨1animal-db🐶",
		normalized: "_1animal-db",
		err:        nil,
	},
	{
		service:    "1🐨1animal-db🐶",
		normalized: "1_1animal-db",
		err:        nil,
	},
	{
		service:    "Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.Too$Long$.",
		normalized: "too_long_.too_long_.too_long_.too_long_.too_long_.too_long_.too_long_.too_long_.too_long_.too_long_.",
		err:        ErrTooLong,
	},
	{
		service:    "bad$service",
		normalized: "bad_service",
		err:        nil,
	},
}

func TestNormalizeService(t *testing.T) {
	cases := append(svcNormCases, serviceNormalizationCase{
		service:    "",
		normalized: DefaultServiceName,
		err:        ErrEmpty,
	})
	for _, testCase := range cases {
		out, err := NormalizeService(testCase.service, "")
		assert.Equal(t, testCase.normalized, out)
		assert.Equal(t, testCase.err, err)
	}
}

func TestNormalizePeerService(t *testing.T) {
	cases := append(svcNormCases, serviceNormalizationCase{
		service:    "",
		normalized: "",
		err:        nil,
	})
	for _, testCase := range cases {
		out, err := NormalizePeerService(testCase.service)
		assert.Equal(t, testCase.normalized, out)
		assert.Equal(t, testCase.err, err)
	}
}
