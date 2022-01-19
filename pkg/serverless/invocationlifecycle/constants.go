// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

const (
	// TraceIDHeader is the header containing the traceID
	TraceIDHeader = "x-datadog-trace-id"

	// SpanIDHeader is the header containing the spanID
	SpanIDHeader = "x-datadog-span-id"

	parentIDHeader = "x-datadog-parent-id"
)
