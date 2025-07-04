// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package defaultforwarder implements a component to send payloads to the backend
package defaultforwarder

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def/transaction"
)

// team: agent-metric-pipelines

// Component is the component type.
type Component interface {
	// TODO: (components) When the code of the forwarder will be
	// in /comp/forwarder move the content of forwarder.Forwarder inside this interface.
	Forwarder
}

// Response represents the response from a forwarder submission
type Response struct {
	Domain     string
	Body       []byte
	StatusCode int
	Err        error
}

// Forwarder interface allows packages to send payload to the backend
type Forwarder interface {
	SubmitV1Series(payload transaction.BytesPayloads, extra http.Header) error
	SubmitV1Intake(payload transaction.BytesPayloads, kind transaction.Kind, extra http.Header) error
	SubmitV1CheckRuns(payload transaction.BytesPayloads, extra http.Header) error
	SubmitSeries(payload transaction.BytesPayloads, extra http.Header) error
	SubmitSketchSeries(payload transaction.BytesPayloads, extra http.Header) error
	SubmitHostMetadata(payload transaction.BytesPayloads, extra http.Header) error
	SubmitAgentChecksMetadata(payload transaction.BytesPayloads, extra http.Header) error
	SubmitMetadata(payload transaction.BytesPayloads, extra http.Header) error
	SubmitProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitProcessDiscoveryChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitProcessEventChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitRTProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitRTContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitConnectionChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitOrchestratorChecks(payload transaction.BytesPayloads, extra http.Header, payloadType int) (chan Response, error)
	SubmitOrchestratorManifests(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// Features is a bitmask to enable specific forwarder features
type Features uint8

const (
	// CoreFeatures bitmask to enable specific core features
	CoreFeatures Features = 1 << iota
	// TraceFeatures bitmask to enable specific trace features
	TraceFeatures
	// ProcessFeatures bitmask to enable specific process features
	ProcessFeatures
	// SysProbeFeatures bitmask to enable specific system-probe features
	SysProbeFeatures
)
