// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package header defines HTTP headers known convention used by the Trace Agent and Datadog's APM intake.
package header

const (
	// TraceCount is the header client implementation should fill
	// with the number of traces contained in the payload.
	TraceCount = "X-Datadog-Trace-Count"

	// ProcessTags is a list that contains process tags split by a ','.
	ProcessTags = "X-Datadog-Process-Tags"

	// ContainerID specifies the name of the header which contains the ID of the
	// container where the request originated.
	// Deprecated in favor of Datadog-Entity-ID.
	ContainerID = "Datadog-Container-ID"

	// LocalData specifies the name of the header which contains the local data for Origin Detection.
	// The Local Data is a list that can contain one or two (split by a ',') of either:
	// * "cid-<container-id>" or "ci-<container-id>" for the container ID.
	// * "in-<cgroupv2-inode>" for the cgroupv2 inode.
	// Possible values:
	// * "cid-<container-id>"
	// * "ci-<container-id>,in-<cgroupv2-inode>"
	LocalData = "Datadog-Entity-ID"

	// ExternalData is a list that contain prefixed-items, split by a ','. Current items are:
	// * "it-<init>" if the container is an init container.
	// * "cn-<container-name>" for the container name.
	// * "pu-<pod-uid>" for the pod UID.
	// Order does not matter.
	// Possible values:
	// * "it-false,cn-nginx,pu-3413883c-ac60-44ab-96e0-9e52e4e173e2"
	// * "cn-init,pu-cb4aba1d-0129-44f1-9f1b-b4dc5d29a3b3,it-true"
	ExternalData = "Datadog-External-Env"

	// Lang specifies the name of the header which contains the language from
	// which the traces originate.
	Lang = "Datadog-Meta-Lang"

	// LangVersion specifies the name of the header which contains the origin
	// language's version.
	LangVersion = "Datadog-Meta-Lang-Version"

	// LangInterpreter specifies the name of the HTTP header containing information
	// about the language interpreter, where applicable.
	LangInterpreter = "Datadog-Meta-Lang-Interpreter"

	// LangInterpreterVendor specifies the name of the HTTP header containing information
	// about the language interpreter vendor, where applicable.
	LangInterpreterVendor = "Datadog-Meta-Lang-Interpreter-Vendor"

	// TracerVersion specifies the name of the header which contains the version
	// of the tracer sending the payload.
	TracerVersion = "Datadog-Meta-Tracer-Version"

	// ComputedTopLevel specifies that the client has marked top-level spans, when set.
	// Any value other than 0, f, F, FALSE, False, false will mean 'yes'.
	ComputedTopLevel = "Datadog-Client-Computed-Top-Level"

	// ComputedStats specifies whether the client has computed stats so that the agent
	// doesn't have to.
	// Any value other than 0, f, F, FALSE, False, false will mean 'yes'.
	ComputedStats = "Datadog-Client-Computed-Stats"

	// DroppedP0Traces contains the number of P0 trace chunks dropped by the client.
	// This value is used to adjust priority rates computed by the agent.
	DroppedP0Traces = "Datadog-Client-Dropped-P0-Traces"

	// DroppedP0Spans contains the number of P0 spans dropped by the client.
	// This value is used for metrics and could be used in the future to adjust priority rates.
	DroppedP0Spans = "Datadog-Client-Dropped-P0-Spans"

	// RatesPayloadVersion contains the version of sampling rates.
	// If both agent and client have the same version, the agent won't return rates in API response.
	RatesPayloadVersion = "Datadog-Rates-Payload-Version"

	// SendRealHTTPStatus can be sent by the client to signal to the agent that
	// it wants to receive the "real" status in the response. By default, the agent
	// will send a 200 OK response for every payload, even those dropped due to
	// intake limits.
	// Any value other than 0, f, F, FALSE, False, false set in this header will cause the agent to send a 429 code to a client
	// when the payload cannot be submitted.
	SendRealHTTPStatus = "Datadog-Send-Real-Http-Status"

	// TracerObfuscationVersion specifies the version of obfuscation done at the tracer, if any.
	// This used to avoid "double obfuscating" data.
	TracerObfuscationVersion = "Datadog-Obfuscation-Version"
)
