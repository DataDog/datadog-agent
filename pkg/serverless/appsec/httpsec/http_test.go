// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpsec

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestParseBodyJson(t *testing.T) {
	rawBody := "{ \"foo\": 1337 }"
	payload := parseBody(
		map[string][]string{
			"content-type": {"application/json;charset=utf-8"},
		},
		&rawBody,
	)

	require.Equal(t, map[string]any{
		"foo": 1337., // JSON numbers are float64 in go
	}, payload)
}

func TestParseBodyUrlEncoded(t *testing.T) {
	rawBody := "foo=1337&bar=b%20a%20z"
	payload := parseBody(
		map[string][]string{
			"content-type": {"application/x-www-form-urlencoded"},
		},
		&rawBody,
	)

	require.Equal(t, map[string][]string{"foo": {"1337"}, "bar": {"b a z"}}, payload)
}

func TestParseBodyMultipartFormData(t *testing.T) {
	rawBody := strings.Join(
		[]string{
			"--B0UND4RY",
			"Content-Disposition: form-data; name=\"foo\"",
			"",
			"1337",
			"--B0UND4RY",
			"Content-Disposition: form-data; name=\"file1\"; filename=\"a.txt\"",
			"Content-Type: text/plain",
			"",
			"Content of a.txt.",
			"",
			"--B0UND4RY",
			"Content-Disposition: form-data; name=\"file2\"; filename=\"a.json\"",
			"Content-Type: application/json",
			"",
			"{ \"foo\": 1337, \"bar\": \"baz\" }",
			"--B0UND4RY",
			"Content-Disposition: form-data; name=\"broken\"; filename=\"bad.json\"",
			"Content-Type: application/vnd.api+json",
			"",
			"{ invalid: true }", // Intentionally not valid JSON
			"--B0UND4RY--",
			"",
		}, "\r\n",
	)
	payload := parseBody(
		map[string][]string{
			"content-type": {"multipart/form-data; boundary=B0UND4RY"},
		},
		&rawBody,
	)

	require.Equal(t, map[string]any{
		"foo":   map[string]any{"data": nil},
		"file1": map[string]any{"filename": "a.txt", "data": "Content of a.txt.\r\n"},
		"file2": map[string]any{
			"filename": "a.json",
			"data": map[string]any{
				"foo": 1337.,
				"bar": "baz",
			},
		},
		"broken": map[string]any{"filename": "bad.json", "data": nil},
	}, payload)
}
