// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_http

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

func TestBuildFormDataDataURIMimeType(t *testing.T) {
	datauri := "datauri"
	formCredentials := &privateconnection.PrivateCredentials{
		HttpDetails: privateconnection.HttpDetails{
			Body: privateconnection.HttpDetailsBody{
				Content:     "[]",
				ContentType: "multipart/form-data",
			},
		},
	}

	tests := []struct {
		name    string
		fields  []FormDataField
		wantErr string
	}{
		{
			name:   "type and subtype",
			fields: []FormDataField{{Key: "file", Value: "data:image/png;base64,aGVsbG8=", Type: &datauri}},
		},
		{
			name:   "surrounding whitespace",
			fields: []FormDataField{{Key: "file", Value: "  data:text/plain;base64,aGVsbG8=\n", Type: &datauri}},
		},
		{
			name:   "subtype with extra slash",
			fields: []FormDataField{{Key: "file", Value: "data:application/vnd/x;base64,aGVsbG8=", Type: &datauri}},
		},
		{
			name:   "empty subtype",
			fields: []FormDataField{{Key: "file", Value: "data:image/;base64,aGVsbG8=", Type: &datauri}},
		},
		{
			name:   "empty type",
			fields: []FormDataField{{Key: "file", Value: "data:/png;base64,aGVsbG8=", Type: &datauri}},
		},
		{
			name:    "no slash",
			fields:  []FormDataField{{Key: "file", Value: "data:png;base64,aGVsbG8=", Type: &datauri}},
			wantErr: "Invalid mime type in data URI for file",
		},
		{
			name:    "empty mime type",
			fields:  []FormDataField{{Key: "file", Value: "data:;base64,aGVsbG8=", Type: &datauri}},
			wantErr: "Invalid mime type in data URI for file",
		},
		{
			name: "invalid field after valid ones",
			fields: []FormDataField{
				{Key: "name", Value: "report"},
				{Key: "file", Value: "data:image/png;base64,aGVsbG8=", Type: &datauri},
				{Key: "thumb", Value: "data:png;base64,aGVsbG8=", Type: &datauri},
			},
			wantErr: "Invalid mime type in data URI for thumb",
		},
	}

	for _, credentials := range []struct {
		name  string
		value *privateconnection.PrivateCredentials
	}{
		{name: "no connection", value: nil},
		{name: "connection form data", value: formCredentials},
	} {
		for _, tt := range tests {
			t.Run(credentials.name+"/"+tt.name, func(t *testing.T) {
				var (
					body        *bytes.Buffer
					contentType string
					err         error
				)
				require.NotPanics(t, func() {
					body, contentType, err = buildFormData(tt.fields, credentials.value)
				})
				if tt.wantErr != "" {
					require.EqualError(t, err, tt.wantErr)
					assert.Nil(t, body)
					return
				}
				require.NoError(t, err)
				require.NotNil(t, body)
				assert.Contains(t, contentType, "multipart/form-data")
				assert.Contains(t, body.String(), "aGVsbG8=")
			})
		}
	}
}
