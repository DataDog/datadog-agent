// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ibm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	tokenPayload = "{\"access_token\":\"some data\",\"created_at\":\"2022-03-22T15:35:50.227Z\",\"expires_at\":\"2022-03-22T15:40:50.227Z\",\"expires_in\":3600}"

	// invalid expire date format
	tokenPayloadExpireError = "{\"access_token\":\"some data\",\"created_at\":\"2022-03-22T15:35:50.227Z\",\"expires_at\":\"2022-03-22T15:40\",\"expires_in\":3600}"

	// We only care about the 'id' entry from the metadata endpoint answer.
	metadataPayload = "{\"id\": \"an ID\"}"
)

func TestGetToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "PUT", r.Method)
		assert.Equal(t, tokenEndpoint, r.RequestURI)
		assert.Equal(t, "ibm", r.Header.Get("Metadata-Flavor"))

		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, "{\"expires_in\": 3600}", string(body))

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		io.WriteString(w, tokenPayload)
	}))
	defer ts.Close()

	defer func(url string) { metadataURL = url }(metadataURL)
	metadataURL = ts.URL

	value, expiresAt, err := getToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "some data", value)

	expectedExpiresAt, _ := time.Parse(time.RFC3339, "2022-03-22T15:40:50.227Z")
	assert.Equal(t, expectedExpiresAt, expiresAt)
}

func TestGetTokenError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		io.WriteString(w, "{}")
	}))
	defer ts.Close()

	defer func(url string) { metadataURL = url }(metadataURL)
	metadataURL = ts.URL

	_, _, err := getToken(context.Background())
	require.NotNil(t, err)
}

func TestGetTokenErrorParsingDate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		io.WriteString(w, tokenPayloadExpireError)
	}))
	defer ts.Close()

	defer func(url string) { metadataURL = url }(metadataURL)
	metadataURL = ts.URL

	renewDate := time.Now()
	// passing some time to make sure renewDate is equal to 'expiresAt'
	time.Sleep(100 * time.Millisecond)

	value, expiresAt, err := getToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "some data", value)

	// expiresAt is set to time.Now() when the API returned an invalid expire date.
	assert.True(t, renewDate.Before(expiresAt), fmt.Sprintf("'%s' should be before '%s'", renewDate, expiresAt))
}

func TestGetHostAliases(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PUT":
			assert.Equal(t, tokenEndpoint, r.RequestURI)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			io.WriteString(w, tokenPayload)
		case "GET":
			assert.Equal(t, instanceEndpoint, r.RequestURI)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			io.WriteString(w, metadataPayload)
		}
	}))
	defer ts.Close()

	defer func(url string) { metadataURL = url }(metadataURL)
	metadataURL = ts.URL

	aliases, err := GetHostAliases(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"an ID"}, aliases)
}
