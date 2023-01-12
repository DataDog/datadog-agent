// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// package shared defines shared utilities used in the implementation of API components.
package shared

const (
	// HeaderTraceCount is the header client implementation should fill
	// with the number of traces contained in the payload.
	HeaderTraceCount = "X-Datadog-Trace-Count"

	// HeaderContainerID specifies the name of the header which contains the ID of the
	// container where the request originated.
	HeaderContainerID = "Datadog-Container-ID"

	// HeaderLang specifies the name of the header which contains the language from
	// which the traces originate.
	HeaderLang = "Datadog-Meta-Lang"

	// HeaderLangVersion specifies the name of the header which contains the origin
	// language's version.
	HeaderLangVersion = "Datadog-Meta-Lang-Version"

	// HeaderLangInterpreter specifies the name of the HTTP header containing information
	// about the language interpreter, where applicable.
	HeaderLangInterpreter = "Datadog-Meta-Lang-Interpreter"

	// HeaderLangInterpreterVendor specifies the name of the HTTP header containing information
	// about the language interpreter vendor, where applicable.
	HeaderLangInterpreterVendor = "Datadog-Meta-Lang-Interpreter-Vendor"

	// HeaderTracerVersion specifies the name of the header which contains the version
	// of the tracer sending the payload.
	HeaderTracerVersion = "Datadog-Meta-Tracer-Version"

	// HeaderComputedTopLevel specifies that the client has marked top-level spans, when set.
	// Any non-empty value will mean 'yes'.
	HeaderComputedTopLevel = "Datadog-Client-Computed-Top-Level"

	// HeaderComputedStats specifies whether the client has computed stats so that the agent
	// doesn't have to.
	HeaderComputedStats = "Datadog-Client-Computed-Stats"

	// HeaderDroppedP0Traces contains the number of P0 trace chunks dropped by the client.
	// This value is used to adjust priority rates computed by the agent.
	HeaderDroppedP0Traces = "Datadog-Client-Dropped-P0-Traces"

	// HeaderDroppedP0Spans contains the number of P0 spans dropped by the client.
	// This value is used for metrics and could be used in the future to adjust priority rates.
	HeaderDroppedP0Spans = "Datadog-Client-Dropped-P0-Spans"

	// HeaderRatesPayloadVersion contains the version of sampling rates.
	// If both agent and client have the same version, the agent won't return rates in API response.
	HeaderRatesPayloadVersion = "Datadog-Rates-Payload-Version"

	// TagContainersTags specifies the name of the tag which holds key/value
	// pairs representing information about the container (Docker, EC2, etc).
	TagContainersTags = "_dd.tags.container"
)
