// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package server

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/stretchr/testify/require"
)

func TestRequestMetadataThroughPayloadAPI(t *testing.T) {
	fi, _ := InitialiseForTests(t, WithAddress("127.0.0.1:0"))
	t.Cleanup(func() { require.NoError(t, fi.Stop()) })
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	_, err := gzipWriter.Write([]byte(`[{"message":"observed"}]`))
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())
	request, err := http.NewRequest(http.MethodPost, fi.URL()+"/api/v2/logs?probe=raw%2Fvalue", bytes.NewReader(compressed.Bytes()))
	require.NoError(t, err)
	request.Host = "intake.example"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Encoding", "gzip")
	request.Header["X-Values"] = []string{"one", "two"}
	request.ContentLength = -1
	request.TransferEncoding = []string{"chunked"}
	request.Trailer = http.Header{"X-End": []string{"complete"}}
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)

	raw, err := client.NewClient(fi.URL()).GetRawPayloads("/api/v2/logs")
	require.NoError(t, err)
	require.Len(t, raw, 1)
	require.Equal(t, compressed.Bytes(), raw[0].Data)
	require.Equal(t, "gzip", raw[0].Encoding)
	require.NotNil(t, raw[0].Request)
	metadata := raw[0].Request
	require.Equal(t, "POST", metadata.Method)
	require.Equal(t, "/api/v2/logs?probe=raw%2Fvalue", metadata.Target)
	require.Equal(t, "/api/v2/logs", metadata.Route)
	require.Equal(t, "intake.example", metadata.Host)
	require.Equal(t, "HTTP/1.1", metadata.Protocol)
	require.Equal(t, []string{"one", "two"}, metadata.Headers.Values("X-Values"))
	require.Equal(t, []string{"complete"}, metadata.Trailers.Values("X-End"))
	require.Equal(t, int64(-1), metadata.ContentLength)
	require.Equal(t, []string{"chunked"}, metadata.TransferEncoding)

	response, err = http.Get(fi.URL() + "/fakeintake/payloads?endpoint=/api/v2/logs&format=json")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var parsed api.APIFakeIntakePayloadsJsonGETResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&parsed))
	require.Len(t, parsed.Payloads, 1)
	require.Equal(t, raw[0].Timestamp, parsed.Payloads[0].Timestamp)
	require.Equal(t, metadata, parsed.Payloads[0].Request)
}
