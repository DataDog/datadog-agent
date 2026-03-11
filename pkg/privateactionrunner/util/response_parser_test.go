// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"mime/multipart"
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseResponseBody(t *testing.T) {
	t.Run("empty data returns empty string", func(t *testing.T) {
		result, err := ParseResponseBody("text/plain", []byte{}, "", "", 200)
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("JSON content type", func(t *testing.T) {
		data := []byte(`{"key": "value", "number": 42}`)
		result, err := ParseResponseBody("application/json", data, "", "", 200)
		require.NoError(t, err)

		m, ok := result.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "value", m["key"])
		assert.Equal(t, float64(42), m["number"])
	})

	t.Run("JSON array", func(t *testing.T) {
		data := []byte(`[1, 2, 3]`)
		result, err := ParseResponseBody("application/json", data, "", "", 200)
		require.NoError(t, err)

		arr, ok := result.([]interface{})
		require.True(t, ok)
		assert.Len(t, arr, 3)
	})

	t.Run("invalid JSON returns raw string", func(t *testing.T) {
		data := []byte(`not valid json`)
		result, err := ParseResponseBody("application/json", data, "", "", 200)
		require.NoError(t, err)
		assert.Equal(t, "not valid json", result)
	})

	t.Run("form URL encoded", func(t *testing.T) {
		data := []byte("key1=value1&key2=value2")
		result, err := ParseResponseBody("application/x-www-form-urlencoded", data, "", "", 200)
		require.NoError(t, err)

		m, ok := result.(map[string]string)
		require.True(t, ok)
		assert.Equal(t, "value1", m["key1"])
		assert.Equal(t, "value2", m["key2"])
	})

	t.Run("plain text", func(t *testing.T) {
		data := []byte("Hello, World!")
		result, err := ParseResponseBody("text/plain", data, "", "", 200)
		require.NoError(t, err)
		assert.Equal(t, "Hello, World!", result)
	})

	t.Run("explicit raw parsing", func(t *testing.T) {
		data := []byte(`{"key": "value"}`)
		result, err := ParseResponseBody("application/json", data, Raw, "", 200)
		require.NoError(t, err)
		// Should not parse JSON, return as raw string
		assert.Equal(t, `{"key": "value"}`, result)
	})

	t.Run("explicit JSON parsing", func(t *testing.T) {
		data := []byte(`{"key": "value"}`)
		result, err := ParseResponseBody("text/plain", data, JSON, "", 200)
		require.NoError(t, err)

		m, ok := result.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "value", m["key"])
	})

	t.Run("binary content returns base64", func(t *testing.T) {
		data := []byte{0x00, 0x01, 0x02, 0x03}
		result, err := ParseResponseBody("application/octet-stream", data, Raw, "", 200)
		require.NoError(t, err)

		encoded := base64.StdEncoding.EncodeToString(data)
		assert.Equal(t, encoded, result)
	})

	t.Run("image content returns base64", func(t *testing.T) {
		data := []byte{0x89, 0x50, 0x4E, 0x47} // PNG header
		result, err := ParseResponseBody("image/png", data, Raw, "", 200)
		require.NoError(t, err)

		encoded := base64.StdEncoding.EncodeToString(data)
		assert.Equal(t, encoded, result)
	})

	t.Run("explicit hex encoding", func(t *testing.T) {
		data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
		result, err := ParseResponseBody("text/plain", data, Raw, "hex", 200)
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(data), result)
	})

	t.Run("auto-detect content type when empty", func(t *testing.T) {
		data := []byte("plain text content")
		result, err := ParseResponseBody("", data, "", "", 200)
		require.NoError(t, err)
		assert.Equal(t, "plain text content", result)
	})

	t.Run("JSON empty string returns empty map", func(t *testing.T) {
		result, err := ParseResponseBody("application/json", []byte(""), JSON, "", 200)
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("content with charset", func(t *testing.T) {
		data := []byte("Hello")
		result, err := ParseResponseBody("text/plain; charset=utf-8", data, "", "", 200)
		require.NoError(t, err)
		assert.Equal(t, "Hello", result)
	})

	t.Run("latin1 charset", func(t *testing.T) {
		data := []byte{0xC0, 0xC1, 0xC2} // Latin1 characters
		result, err := ParseResponseBody("text/plain; charset=iso-8859-1", data, "", "", 200)
		require.NoError(t, err)
		assert.NotEmpty(t, result)
	})

	t.Run("unsupported encoding returns error", func(t *testing.T) {
		data := []byte("Hello")
		_, err := ParseResponseBody("text/plain", data, Raw, "unsupported-encoding", 200)
		assert.Error(t, err)
	})
}

func TestParseResponseBodyFormData(t *testing.T) {
	// Create a multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add a text field
	err := writer.WriteField("name", "John Doe")
	require.NoError(t, err)

	// Add another text field
	err = writer.WriteField("email", "john@example.com")
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	contentType := writer.FormDataContentType()
	result, err := ParseResponseBody(contentType, buf.Bytes(), "", "", 200)
	require.NoError(t, err)

	fields, ok := result.([]FormDataField)
	require.True(t, ok)
	require.Len(t, fields, 2)

	// Check first field
	assert.Equal(t, "name", fields[0].Name)
	assert.Equal(t, "John Doe", fields[0].Data)

	// Check second field
	assert.Equal(t, "email", fields[1].Name)
	assert.Equal(t, "john@example.com", fields[1].Data)
}

func TestParseResponseBodyFormDataWithFile(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Create a file field with custom headers
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="test.txt"`)
	h.Set("Content-Type", "text/plain")

	part, err := writer.CreatePart(h)
	require.NoError(t, err)
	_, err = part.Write([]byte("file contents"))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	contentType := writer.FormDataContentType()
	result, err := ParseResponseBody(contentType, buf.Bytes(), "", "", 200)
	require.NoError(t, err)

	fields, ok := result.([]FormDataField)
	require.True(t, ok)
	require.Len(t, fields, 1)

	assert.Equal(t, "file", fields[0].Name)
	assert.Equal(t, "test.txt", fields[0].Filename)
	assert.Equal(t, "text/plain", fields[0].Type)
	assert.Equal(t, "file contents", fields[0].Data)
}

func TestEncodings(t *testing.T) {
	tests := []struct {
		name     string
		encoding string
		data     []byte
		expected string
	}{
		{
			name:     "ascii",
			encoding: "ascii",
			data:     []byte("Hello"),
			expected: "Hello",
		},
		{
			name:     "utf8",
			encoding: "utf8",
			data:     []byte("Hello"),
			expected: "Hello",
		},
		{
			name:     "utf-8",
			encoding: "utf-8",
			data:     []byte("Hello"),
			expected: "Hello",
		},
		{
			name:     "base64",
			encoding: "base64",
			data:     []byte("Hello"),
			expected: base64.StdEncoding.EncodeToString([]byte("Hello")),
		},
		{
			name:     "base64url",
			encoding: "base64url",
			data:     []byte("Hello"),
			expected: base64.URLEncoding.EncodeToString([]byte("Hello")),
		},
		{
			name:     "hex",
			encoding: "hex",
			data:     []byte{0xDE, 0xAD, 0xBE, 0xEF},
			expected: "deadbeef",
		},
		{
			name:     "binary",
			encoding: "binary",
			data:     []byte("Hello"),
			expected: "Hello",
		},
		{
			name:     "latin1",
			encoding: "latin1",
			data:     []byte{0xC0, 0xC1},
			expected: "ÀÁ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseResponseBody("text/plain", tt.data, Raw, tt.encoding, 200)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUTF16LEEncoding(t *testing.T) {
	// UTF-16LE encoded "Hi" (H = 0x0048, i = 0x0069)
	data := []byte{0x48, 0x00, 0x69, 0x00}
	result, err := ParseResponseBody("text/plain", data, Raw, "utf16le", 200)
	require.NoError(t, err)
	assert.Equal(t, "Hi", result)
}

func TestInferResponseParsing(t *testing.T) {
	tests := []struct {
		contentType string
		data        []byte
		isJSON      bool
	}{
		{
			contentType: "application/json",
			data:        []byte(`{"test": true}`),
			isJSON:      true,
		},
		{
			contentType: "text/html",
			data:        []byte("<html></html>"),
			isJSON:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result, err := ParseResponseBody(tt.contentType, tt.data, "", "", 200)
			require.NoError(t, err)

			if tt.isJSON {
				_, ok := result.(map[string]interface{})
				assert.True(t, ok)
			} else {
				_, ok := result.(string)
				assert.True(t, ok)
			}
		})
	}
}
