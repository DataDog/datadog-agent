// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode/utf16"
)

const (
	Raw            string = "raw"
	JSON           string = "json"
	FormData       string = "form-data"
	FormURLEncoded string = "form-urlencoded"
)

var CharsetAliases = map[string]string{
	"iso-8859-1": "latin1",
	"iso88591":   "latin1",
	"iso8859-1":  "latin1",
}

var Encoding = []string{"ascii", "utf8", "utf-8", "utf16le", "ucs2", "ucs-2", "base64", "base64url", "latin1", "binary", "hex"}

type FormDataField struct {
	Filename string
	Name     string
	Type     string
	Data     interface{}
}

func ParseResponseBody(
	contentType string,
	data []byte,
	responseParsing string,
	responseEncoding string,
	statusCode int,
) (interface{}, error) {
	if !isBuffer(data) {
		return data, nil
	}
	if len(data) == 0 {
		return "", nil
	}
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	mimeType, mimeParams, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	if responseParsing == "" {
		responseParsing = inferResponseParsing(mimeType)
	}

	if responseParsing == FormData {
		return parseFormData(mimeParams, data)
	}

	encoding := responseEncoding
	if encoding == "" {
		encoding = inferEncoding(mimeType, mimeParams, responseParsing)
	}

	stringOutput, err := encodeToString(data, encoding)
	if err != nil {
		return data, err
	}
	switch responseParsing {
	case Raw:
		return stringOutput, nil
	case JSON:
		if stringOutput == "" {
			return map[string]interface{}{}, nil
		}
		var result interface{}
		if err := json.Unmarshal([]byte(stringOutput), &result); err != nil {
			// we do not error out here because a number of APIs do not return valid JSON.
			return stringOutput, nil
		}
		return result, nil
	case FormURLEncoded:
		return parseUrlEncoded(stringOutput)
	default:
		return stringOutput, nil
	}
}

// EncodeToString converts []byte to string using given encoding
func encodeToString(data []byte, encoding string) (string, error) {
	switch encoding {
	case "ascii":
		return string(data), nil
	case "utf8", "utf-8":
		return string(data), nil
	case "utf16le", "ucs2", "ucs-2":
		// ucs2 is a subset of utf-16
		var uint16Slice []uint16
		for i := 0; i < len(data); i += 2 {
			uint16Slice = append(uint16Slice, binary.LittleEndian.Uint16(data[i:i+2]))
		}
		// Decode UTF-16LE to string
		return string(utf16.Decode(uint16Slice)), nil
	case "base64":
		return base64.StdEncoding.EncodeToString(data), nil
	case "base64url":
		return base64.URLEncoding.EncodeToString(data), nil
	case "latin1":
		var res []rune
		for _, b := range data {
			res = append(res, rune(b))
		}
		return string(res), nil
	case "binary":
		return string(data), nil
	case "hex":
		return hex.EncodeToString(data), nil
	default:
		return "", fmt.Errorf("unsupported encoding: %s", encoding)
	}
}

func inferResponseParsing(contentType string) string {
	switch contentType {
	case "application/json":
		return JSON
	case "application/x-www-form-urlencoded":
		return FormURLEncoded
	case "multipart/form-data":
		return FormData
	default:
		return Raw
	}
}

func parseUrlEncoded(str string) (map[string]string, error) {
	result := make(map[string]string)
	values, err := url.ParseQuery(str)
	if err != nil {
		return result, err
	}
	for key, value := range values {
		result[key] = value[0]
	}
	return result, nil
}

func parseFormData(params map[string]string, data []byte) ([]FormDataField, error) {
	return parseMultipart(data, params["boundary"])
}

func isEncoding(encoding string) bool {
	return slices.Contains(Encoding, encoding)
}

func getEncodingFromMIME(params map[string]string) string {
	charset := params["charset"]
	if encoding, ok := CharsetAliases[charset]; ok {
		if isEncoding(encoding) {
			return encoding
		}
	}
	return ""
}

func inferEncoding(
	mimeType string,
	mimeParams map[string]string,
	responseType string,
) string {
	textParsing := []string{FormData, FormURLEncoded, JSON}
	for _, tp := range textParsing {
		if responseType == tp {
			mimeEncoding := getEncodingFromMIME(mimeParams)
			if mimeEncoding == "" {
				return "utf-8"
			}
			return mimeEncoding
		}
	}
	if mimeType == "" {
		return "utf-8"
	}
	binaryTypes := []string{"video", "image", "audio", "font", "model"}
	for _, bt := range binaryTypes {
		if strings.Contains(mimeType, bt) {
			return "base64"
		}
	}
	if mimeType == "application/octet-stream" {
		return "base64"
	}

	if mimeEncoding := getEncodingFromMIME(mimeParams); mimeEncoding != "" {
		return mimeEncoding
	}
	return "utf-8"
}

func parseMultipart(data []byte, boundary string) ([]FormDataField, error) {
	var fields []FormDataField
	parts := multipart.NewReader(bytes.NewReader(data), boundary)
	for {
		part, err := parts.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fields, err
		}

		formDataField := FormDataField{
			Filename: part.FileName(),
			Name:     part.FormName(),
			Type:     part.Header.Get("Content-Type"),
		}
		if formDataField.Type == "" {
			// set default form data to text/plain. https://www.ietf.org/rfc/rfc2388.txt
			formDataField.Type = "text/plain"
		}
		contentBytes, err := io.ReadAll(part)
		if err != nil {
			return fields, err
		}
		data, err := ParseResponseBody(formDataField.Type, contentBytes, "", "", 0)
		if err != nil {
			return fields, err
		}
		formDataField.Data = data
		fields = append(fields, formDataField)
	}
	return fields, nil
}

func isBuffer(data interface{}) bool {
	if _, ok := data.([]byte); ok {
		return true
	}
	return false
}
