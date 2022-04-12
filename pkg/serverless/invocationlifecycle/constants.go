// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import "github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"

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

	// ParentIDHeader is the header containing the parentID
	// used in  /trace.go
	ParentIDHeader = "x-datadog-parent-id"
)

// InferredSpansEnabled is used to determine if we need to make
// and inferred span or not
var InferredSpansEnabled = inferredspan.InferredSpansEnabled
