// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package mock provides mock implementation for testing
package mock

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def/transaction"
)

// MockComponent is a mock implementation of the Component interface
type MockComponent struct{}

// NewMock returns a new mock component
func NewMock() def.Mock {
	return &MockComponent{}
}

// SubmitV1Series implements def.Forwarder
func (m *MockComponent) SubmitV1Series(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

// SubmitV1Intake implements def.Forwarder
func (m *MockComponent) SubmitV1Intake(payload transaction.BytesPayloads, kind transaction.Kind, extra http.Header) error {
	return nil
}

// SubmitV1CheckRuns implements def.Forwarder
func (m *MockComponent) SubmitV1CheckRuns(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

// SubmitSeries implements def.Forwarder
func (m *MockComponent) SubmitSeries(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

// SubmitSketchSeries implements def.Forwarder
func (m *MockComponent) SubmitSketchSeries(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

// SubmitHostMetadata implements def.Forwarder
func (m *MockComponent) SubmitHostMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

// SubmitAgentChecksMetadata implements def.Forwarder
func (m *MockComponent) SubmitAgentChecksMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

// SubmitMetadata implements def.Forwarder
func (m *MockComponent) SubmitMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return nil
}

// SubmitProcessChecks implements def.Forwarder
func (m *MockComponent) SubmitProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan def.Response, error) {
	return make(chan def.Response), nil
}

// SubmitProcessDiscoveryChecks implements def.Forwarder
func (m *MockComponent) SubmitProcessDiscoveryChecks(payload transaction.BytesPayloads, extra http.Header) (chan def.Response, error) {
	return make(chan def.Response), nil
}

// SubmitProcessEventChecks implements def.Forwarder
func (m *MockComponent) SubmitProcessEventChecks(payload transaction.BytesPayloads, extra http.Header) (chan def.Response, error) {
	return make(chan def.Response), nil
}

// SubmitRTProcessChecks implements def.Forwarder
func (m *MockComponent) SubmitRTProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan def.Response, error) {
	return make(chan def.Response), nil
}

// SubmitContainerChecks implements def.Forwarder
func (m *MockComponent) SubmitContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan def.Response, error) {
	return make(chan def.Response), nil
}

// SubmitRTContainerChecks implements def.Forwarder
func (m *MockComponent) SubmitRTContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan def.Response, error) {
	return make(chan def.Response), nil
}

// SubmitConnectionChecks implements def.Forwarder
func (m *MockComponent) SubmitConnectionChecks(payload transaction.BytesPayloads, extra http.Header) (chan def.Response, error) {
	return make(chan def.Response), nil
}

// SubmitOrchestratorChecks implements def.Forwarder
func (m *MockComponent) SubmitOrchestratorChecks(payload transaction.BytesPayloads, extra http.Header, payloadType int) (chan def.Response, error) {
	return make(chan def.Response), nil
}

// SubmitOrchestratorManifests implements def.Forwarder
func (m *MockComponent) SubmitOrchestratorManifests(payload transaction.BytesPayloads, extra http.Header) (chan def.Response, error) {
	return make(chan def.Response), nil
}
