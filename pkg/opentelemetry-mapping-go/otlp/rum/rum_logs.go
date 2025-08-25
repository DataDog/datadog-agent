// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package rum

import (
	"net/http"

	"go.opentelemetry.io/collector/pdata/plog"
	semconv "go.opentelemetry.io/otel/semconv/v1.5.0"
)

// ToLogs converts a RUM payload to OpenTelemetry Logs
func ToLogs(payload map[string]any, req *http.Request) plog.Logs {
	results := plog.NewLogs()
	rl := results.ResourceLogs().AppendEmpty()
	rl.SetSchemaUrl(semconv.SchemaURL)
	rl.Resource().Attributes().PutStr(string(semconv.ServiceNameKey), "browser-rum-sdk")
	parseDDForwardIntoResource(rl.Resource().Attributes(), req.URL.Query().Get("ddforward"))

	in := rl.ScopeLogs().AppendEmpty()
	in.Scope().SetName(instrumentationScopeName)

	newLogRecord := in.LogRecords().AppendEmpty()

	flatPayload := flattenJSON(payload)

	setOTLPAttributes(flatPayload, newLogRecord.Attributes())

	return results
}
