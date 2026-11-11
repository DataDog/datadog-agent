// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !aix && !freebsd && !solaris

package datadogextension

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
)

func diRecord(name, payload string) plog.LogRecord {
	r := plog.NewLogRecord()
	r.SetEventName(name)
	r.Attributes().PutInt("datadog.di.schema_version", 1)
	r.Attributes().PutStr("datadog.di.payload", payload)
	r.Attributes().PutStr("datadog.di.tags", "env:test,version:one")
	return r
}

func TestDIEnvelopeConversionPreservesCapture(t *testing.T) {
	payload := `{"debugger":{"snapshot":{"value":1267650600228229401496703205376,"redacted":"[REDACTED]","missing":null}}}`
	for _, name := range []string{"dd.debugger.snapshot", "dd.debugger.log"} {
		body, contentType, err := encodeDIEvent(diRecord(name, payload))
		require.NoError(t, err)
		require.Equal(t, "application/json", contentType)
		require.Equal(t, "["+payload+"]", string(body))
	}
}

func TestDIDiagnosticUsesMultipartIntakeProtocol(t *testing.T) {
	payload := `{"type":"diagnostic","debugger":{"diagnostics":{"status":"INSTALLED"}}}`
	body, contentType, err := encodeDIEvent(diRecord("dd.debugger.diagnostic", payload))
	require.NoError(t, err)
	_, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	event, err := reader.NextPart()
	require.NoError(t, err)
	require.Equal(t, "event", event.FormName())
	require.Equal(t, "application/json", event.Header.Get("Content-Type"))
	data, err := io.ReadAll(event)
	require.NoError(t, err)
	require.Equal(t, "["+payload+"]", string(data))
}

func TestDIInvalidEnvelopesAreRejected(t *testing.T) {
	for _, r := range []plog.LogRecord{diRecord("unknown", "{}"), diRecord("dd.debugger.snapshot", "[]"), diRecord("dd.debugger.snapshot", "invalid"), diRecord("dd.debugger.symbols", "{}"), plog.NewLogRecord()} {
		_, _, err := encodeDIEvent(r)
		require.Error(t, err)
	}
	r := diRecord("dd.debugger.snapshot", "{}")
	r.Attributes().PutInt("datadog.di.schema_version", 2)
	_, _, err := encodeDIEvent(r)
	require.Error(t, err)
}

func TestDISymbolMultipart(t *testing.T) {
	payload := `{"event":{"runtimeId":"runtime-one","batchNum":1},"symbols":{"scopes":[{"name":"health"}],"large":1267650600228229401496703205376}}`
	body, contentType, err := encodeDIEvent(diRecord("dd.debugger.symbols", payload))
	require.NoError(t, err)
	_, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	event, err := reader.NextPart()
	require.NoError(t, err)
	require.Equal(t, "event", event.FormName())
	var metadata map[string]any
	require.NoError(t, json.NewDecoder(event).Decode(&metadata))
	file, err := reader.NextPart()
	require.NoError(t, err)
	require.Equal(t, "file", file.FormName())
	compressed, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, float64(len(compressed)), metadata["attachmentSize"])
	unzip, err := gzip.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	symbols, err := io.ReadAll(unzip)
	require.NoError(t, err)
	require.Contains(t, string(symbols), "1267650600228229401496703205376")
	require.NoError(t, unzip.Close())
}

func TestDIIntakeDeliveryAndDisabledGate(t *testing.T) {
	count := 0
	intake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		require.Equal(t, "/api/v2/debugger", r.URL.Path)
		require.Equal(t, "collector-key", r.Header.Get("DD-API-KEY"))
		require.Equal(t, "env:test,version:one", r.URL.Query().Get("ddtags"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.JSONEq(t, `[{"message":"capture"}]`, string(body))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer intake.Close()
	logs := plog.NewLogs()
	diRecord("dd.debugger.snapshot", `{"message":"capture"}`).CopyTo(logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty())
	e := &datadogExtension{}
	require.Error(t, e.ConsumeDIEvents(t.Context(), logs))
	require.Zero(t, count)
	e.di = &diSender{client: intake.Client(), endpoint: intake.URL + "/api/v2/debugger", key: "collector-key"}
	require.NoError(t, e.ConsumeDIEvents(t.Context(), logs))
	require.Equal(t, 1, count)
}
