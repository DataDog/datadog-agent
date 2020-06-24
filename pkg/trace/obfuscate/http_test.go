// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package obfuscate

import (
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"

	"github.com/stretchr/testify/assert"
)

// inOutTest is holds a test input and an expected output.
type inOutTest struct{ in, out string }

func TestObfuscateHTTP(t *testing.T) {
	const testURL = "http://foo.com/1/2/3?q=james"

	t.Run("disabled", testHTTPObfuscation(&inOutTest{
		in:  testURL,
		out: testURL,
	}, nil))

	t.Run("query", func(t *testing.T) {
		conf := &config.ObfuscationConfig{HTTP: config.HTTPObfuscationConfig{
			RemoveQueryString: true,
		}}
		for ti, tt := range []inOutTest{
			{
				in:  "http://foo.com/",
				out: "http://foo.com/",
			},
			{
				in:  "http://foo.com/123",
				out: "http://foo.com/123",
			},
			{
				in:  "http://foo.com/id/123/page/1?search=bar&page=2",
				out: "http://foo.com/id/123/page/1?",
			},
			{
				in:  "http://foo.com/id/123/page/1?search=bar&page=2#fragment",
				out: "http://foo.com/id/123/page/1?#fragment",
			},
			{
				in:  "http://foo.com/id/123/page/1?blabla",
				out: "http://foo.com/id/123/page/1?",
			},
			{
				in:  "http://foo.com/id/123/pa%3Fge/1?blabla",
				out: "http://foo.com/id/123/pa%3Fge/1?",
			},
		} {
			t.Run(strconv.Itoa(ti), testHTTPObfuscation(&tt, conf))
		}
	})

	t.Run("digits", func(t *testing.T) {
		conf := &config.ObfuscationConfig{HTTP: config.HTTPObfuscationConfig{
			RemovePathDigits: true,
		}}
		for ti, tt := range []inOutTest{
			{
				in:  "http://foo.com/",
				out: "http://foo.com/",
			},
			{
				in:  "http://foo.com/name?query=search",
				out: "http://foo.com/name?query=search",
			},
			{
				in:  "http://foo.com/id/123/page/1?search=bar&page=2",
				out: "http://foo.com/id/?/page/??search=bar&page=2",
			},
			{
				in:  "http://foo.com/id/a1/page/1qwe233?search=bar&page=2#fragment-123",
				out: "http://foo.com/id/?/page/??search=bar&page=2#fragment-123",
			},
			{
				in:  "http://foo.com/123",
				out: "http://foo.com/?",
			},
			{
				in:  "http://foo.com/123/abcd9",
				out: "http://foo.com/?/?",
			},
			{
				in:  "http://foo.com/123/name/abcd9",
				out: "http://foo.com/?/name/?",
			},
			{
				in:  "http://foo.com/123/name/abcd9",
				out: "http://foo.com/?/name/?",
			},
			{
				in:  "http://foo.com/1%3F3/nam%3Fe/abcd9",
				out: "http://foo.com/?/nam%3Fe/?",
			},
		} {
			t.Run(strconv.Itoa(ti), testHTTPObfuscation(&tt, conf))
		}
	})

	t.Run("both", func(t *testing.T) {
		conf := &config.ObfuscationConfig{HTTP: config.HTTPObfuscationConfig{
			RemoveQueryString: true,
			RemovePathDigits:  true,
		}}
		for ti, tt := range []inOutTest{
			{
				in:  "http://foo.com/",
				out: "http://foo.com/",
			},
			{
				in:  "http://foo.com/name/id",
				out: "http://foo.com/name/id",
			},
			{
				in:  "http://foo.com/name/id?query=search",
				out: "http://foo.com/name/id?",
			},
			{
				in:  "http://foo.com/id/123/page/1?search=bar&page=2",
				out: "http://foo.com/id/?/page/??",
			},
			{
				in:  "http://foo.com/id/123/page/1?search=bar&page=2#fragment",
				out: "http://foo.com/id/?/page/??#fragment",
			},
			{
				in:  "http://foo.com/1%3F3/nam%3Fe/abcd9",
				out: "http://foo.com/?/nam%3Fe/?",
			},
			{
				in:  "http://foo.com/id/123/pa%3Fge/1?blabla",
				out: "http://foo.com/id/?/pa%3Fge/??",
			},
		} {
			t.Run(strconv.Itoa(ti), testHTTPObfuscation(&tt, conf))
		}
	})

	t.Run("wrong-type", func(t *testing.T) {
		assert := assert.New(t)
		span := pb.Span{Type: "web_server", Meta: map[string]string{"http.url": testURL}}
		NewObfuscator(&config.ObfuscationConfig{
			HTTP: config.HTTPObfuscationConfig{
				RemoveQueryString: true,
				RemovePathDigits:  true,
			},
		}).Obfuscate(&span)
		assert.Equal(testURL, span.Meta["http.url"])
	})
}

// testHTTPObfuscation tests that the given input results in the given output using the passed configuration.
func testHTTPObfuscation(tt *inOutTest, conf *config.ObfuscationConfig) func(t *testing.T) {
	return func(t *testing.T) {
		var cfg config.ObfuscationConfig
		if conf != nil {
			cfg = *conf
		}
		assert := assert.New(t)
		span := pb.Span{
			Type: "http",
			Meta: map[string]string{"http.url": tt.in},
		}
		NewObfuscator(&cfg).Obfuscate(&span)
		assert.Equal(tt.out, span.Meta["http.url"])
	}
}
