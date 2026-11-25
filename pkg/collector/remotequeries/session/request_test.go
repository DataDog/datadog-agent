// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package session

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testCrawlID  = "7c4e2b1a-9f3d-4c8e-b6a5-1d2f3e4a5b6c"
	testTaskID   = "d25e7f9f-6883-4462-b31e-b449f70c0894"
	testUploadID = "6f1c2a9e-3b7d-4f0a-9c84-2d5e1b7a0c3f"
)

func executeJSON(edit func(map[string]any)) string {
	req := map[string]any{
		"operation":     "execute",
		"target":        map[string]any{"host": "db.example.com", "port": 1521, "dbname": "FREEPDB1"},
		"query":         "SELECT 1 FROM dual",
		"includeSchema": true,
		"resultDelivery": map[string]any{
			"crawlId": testCrawlID, "taskId": testTaskID, "uploadId": testUploadID,
			"baseUrl": "https://api.datad0g.com/api/unstable/its-agent-intake",
			"limits": map[string]any{
				"maxFileBytes": 16777216, "maxResultBytes": 107374182400, "maxRowBytes": 16777216, "maxColumns": 1024,
				"maxSchemaBytes": 1048576, "maxPages": 1280, "timeoutMs": 60000,
			},
		},
		"traceContext": map[string]any{"traceId": "5843016738114925931", "parentId": "1122334455667788", "samplingPriority": 1},
	}
	if edit != nil {
		edit(req)
	}
	out, _ := json.Marshal(req)
	return string(out)
}

func delivery(req map[string]any) map[string]any { return req["resultDelivery"].(map[string]any) }
func limits(req map[string]any) map[string]any {
	return delivery(req)["limits"].(map[string]any)
}

// REQ-01, REQ-02
func TestREQ02_ValidateARequestBeforeUsingIt(t *testing.T) {
	req, err := ParseRequest(executeJSON(nil))
	require.NoError(t, err)
	assert.Equal(t, Request{
		Operation: OperationExecute,
		Target:    Target{Host: "db.example.com", Port: 1521, DBName: "FREEPDB1"},
		Query:     "SELECT 1 FROM dual", IncludeSchema: true,
		Delivery: &Delivery{CrawlID: testCrawlID, TaskID: testTaskID, UploadID: testUploadID, BaseURL: "https://api.datad0g.com/api/unstable/its-agent-intake",
			Limits: Limits{MaxFileBytes: 16777216, MaxResultBytes: 107374182400, MaxRowBytes: 16777216, MaxColumns: 1024, MaxSchemaBytes: 1048576, MaxPages: 1280, TimeoutMs: 60000}},
		Trace: &Trace{TraceID: "5843016738114925931", ParentID: "1122334455667788", SamplingPriority: 1},
	}, req)

	resolve, err := ParseRequest(`{"operation":"resolve","target":{"database_instance":"orders-db"}}`)
	require.NoError(t, err)
	assert.Equal(t, Request{Operation: OperationResolve, Target: Target{DatabaseInstance: "orders-db"}}, resolve)

	noTrace, err := ParseRequest(executeJSON(func(r map[string]any) { delete(r, "traceContext") }))
	require.NoError(t, err)
	assert.Nil(t, noTrace.Trace)

	for name, raw := range map[string]string{
		"not JSON":                      `{`,
		"not an object":                 `[]`,
		"two objects":                   `{"operation":"resolve","target":{"database_instance":"x"}}{}`,
		"repeated field":                `{"operation":"resolve","operation":"resolve","target":{"database_instance":"x"}}`,
		"unknown operation":             `{"operation":"drop","target":{"database_instance":"x"}}`,
		"resolve with a query":          `{"operation":"resolve","target":{"database_instance":"x"},"query":"SELECT 1"}`,
		"resolve with an empty query":   `{"operation":"resolve","target":{"database_instance":"x"},"query":""}`,
		"database instance with a host": `{"operation":"resolve","target":{"database_instance":"x","host":"h"}}`,
		"database instance with spaces": `{"operation":"resolve","target":{"database_instance":" x"}}`,
		"missing dbname":                `{"operation":"resolve","target":{"host":"h","port":1}}`,
		"unknown field":                 executeJSON(func(r map[string]any) { r["extra"] = 1 }),
		"unknown target field":          executeJSON(func(r map[string]any) { r["target"].(map[string]any)["sid"] = "x" }),
		"empty query":                   executeJSON(func(r map[string]any) { r["query"] = "  " }),
		"query not a string":            executeJSON(func(r map[string]any) { r["query"] = 1 }),
		"missing includeSchema":         executeJSON(func(r map[string]any) { delete(r, "includeSchema") }),
		"null includeSchema":            executeJSON(func(r map[string]any) { r["includeSchema"] = nil }),
		"string includeSchema":          executeJSON(func(r map[string]any) { r["includeSchema"] = "true" }),
		"port zero":                     executeJSON(func(r map[string]any) { r["target"].(map[string]any)["port"] = 0 }),
		"port too large":                executeJSON(func(r map[string]any) { r["target"].(map[string]any)["port"] = 65536 }),
		"port fractional":               executeJSON(func(r map[string]any) { r["target"].(map[string]any)["port"] = 1521.5 }),
		"crawl ID not a UUID":           executeJSON(func(r map[string]any) { delivery(r)["crawlId"] = "x" }),
		"upload ID with a slash":        executeJSON(func(r map[string]any) { delivery(r)["uploadId"] = "a/b" }),
		"upload ID dot dot":             executeJSON(func(r map[string]any) { delivery(r)["uploadId"] = ".." }),
		"base URL with credentials":     executeJSON(func(r map[string]any) { delivery(r)["baseUrl"] = "https://u:p@api.datad0g.com/x" }),
		"base URL with a query":         executeJSON(func(r map[string]any) { delivery(r)["baseUrl"] = "https://api.datad0g.com/x?y=1" }),
		"base URL with a fragment":      executeJSON(func(r map[string]any) { delivery(r)["baseUrl"] = "https://api.datad0g.com/x#y" }),
		"base URL with a space":         executeJSON(func(r map[string]any) { delivery(r)["baseUrl"] = "https://api.datad0g.com/x y" }),
		"base URL with a newline":       executeJSON(func(r map[string]any) { delivery(r)["baseUrl"] = "https://api.datad0g.com/x\r\nX: y" }),
		"base URL of another scheme":    executeJSON(func(r map[string]any) { delivery(r)["baseUrl"] = "ftp://api.datad0g.com/x" }),
		"base URL without a host":       executeJSON(func(r map[string]any) { delivery(r)["baseUrl"] = "https:///x" }),
		"missing limit":                 executeJSON(func(r map[string]any) { delete(limits(r), "maxPages") }),
		"zero limit":                    executeJSON(func(r map[string]any) { limits(r)["maxPages"] = 0 }),
		"fractional limit":              executeJSON(func(r map[string]any) { limits(r)["maxPages"] = 1.5 }),
		"unknown limit":                 executeJSON(func(r map[string]any) { limits(r)["maxRows"] = 1 }),
		"missing delivery field":        executeJSON(func(r map[string]any) { delete(delivery(r), "taskId") }),
		"trace priority 0":              executeJSON(func(r map[string]any) { r["traceContext"].(map[string]any)["samplingPriority"] = 0 }),
		"trace ID zero":                 executeJSON(func(r map[string]any) { r["traceContext"].(map[string]any)["traceId"] = "0" }),
		"trace ID over 64 bits":         executeJSON(func(r map[string]any) { r["traceContext"].(map[string]any)["traceId"] = "18446744073709551616" }),
		"trace ID a number":             executeJSON(func(r map[string]any) { r["traceContext"].(map[string]any)["traceId"] = 1 }),
		"trace missing parent":          executeJSON(func(r map[string]any) { delete(r["traceContext"].(map[string]any), "parentId") }),
	} {
		_, err := ParseRequest(raw)
		assert.Error(t, err, name)
	}
}

// OBS-01
func TestOBS01_AnswerWithOneStableCodeAndASafeMessage(t *testing.T) {
	assert.JSONEq(t, `{"status":"MATCHED"}`, Matched())
	assert.JSONEq(t, `{"status":"SUCCEEDED"}`, Succeeded())
	assert.JSONEq(t, `{"status":"FAILED","error":{"code":"timeout","message":"too slow"}}`, Failed(Fail(CodeTimeout, "too slow")))
	for _, code := range []string{CodeInvalidRequest, CodeTargetNotFound, CodeTargetUnavailable, CodeCredentialsUnavailable, CodeConnectionFailed,
		CodeQueryFailed, CodeUnsupportedColumn, CodeInvalidColumns, CodeSchemaTooLarge, CodeRowTooLarge, CodeResultTooLarge, CodeUploadFailed,
		CodeTimeout, CodeCancelled, CodeInternalError} {
		assert.Regexp(t, `^[a-z][a-z0-9_]{0,63}$`, code, "intake accepts the code as an abort reason")
		assert.False(t, strings.Contains(code, " "))
	}
}
