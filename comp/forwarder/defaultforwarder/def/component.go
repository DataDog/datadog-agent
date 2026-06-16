// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package defaultforwarder defines the interface for the default forwarder component.
package defaultforwarder

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

// team: agent-metric-pipelines

// Response contains the response details of a successfully posted transaction
type Response struct {
	Domain     string
	Body       []byte
	StatusCode int
	Err        error
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

// SetFeature sets forwarder features in a feature set
func SetFeature(features, flag Features) Features { return features | flag }

// ClearFeature clears forwarder features from a feature set
func ClearFeature(features, flag Features) Features { return features &^ flag }

// ToggleFeature toggles forwarder features in a feature set
func ToggleFeature(features, flag Features) Features { return features ^ flag }

// HasFeature lets you know if a specific feature flag is set in a feature set
func HasFeature(features, flag Features) bool { return features&flag != 0 }

// Forwarder interface allows packages to send payload to the backend
type Forwarder interface {
	SubmitV1Series(payload transaction.BytesPayloads, extra http.Header) error
	SubmitV1Intake(payload transaction.BytesPayloads, kind transaction.Kind, extra http.Header) error
	SubmitV1CheckRuns(payload transaction.BytesPayloads, extra http.Header) error
	SubmitHostMetadata(payload transaction.BytesPayloads, extra http.Header) error
	SubmitAgentChecksMetadata(payload transaction.BytesPayloads, extra http.Header) error
	SubmitMetadata(payload transaction.BytesPayloads, extra http.Header) error
	SubmitProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitProcessDiscoveryChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitRTProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitRTContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitConnectionChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error)
	SubmitOrchestratorChecks(payload transaction.BytesPayloads, extra http.Header, payloadType int) error
	SubmitOrchestratorManifests(payload transaction.BytesPayloads, extra http.Header) error
	SubmitV1IntakeDirect(ctx context.Context, payload transaction.BytesPayloads, kind transaction.Kind, extra http.Header) error

	ForwarderV2
}

// ForwarderV2 is a minimalist forwarder interface that is product agnostic.
type ForwarderV2 interface {
	GetDomainResolvers() []resolver.DomainResolver
	SubmitTransaction(*transaction.HTTPTransaction) error
}

// Component is the component type.
type Component interface {
	Forwarder
}
