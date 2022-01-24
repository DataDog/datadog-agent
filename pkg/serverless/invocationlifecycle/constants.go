// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

const (
	// TraceIDHeader is the header containing the traceID
	// used in /trace-context
	TraceIDHeader = "x-datadog-trace-id"

	// SpanIDHeader is the header containing the spanID
	// used in /lambda/start-invocation
	SpanIDHeader = "x-datadog-span-id"

	// InvocationErrorHeader : if set to "true", the extension will know that the current invocation has failed
	// used in /lambda/end-invocation
	InvocationErrorHeader = "x-datadog-invocation-error"

	parentIDHeader = "x-datadog-parent-id"
)
