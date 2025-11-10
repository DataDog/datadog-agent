// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

// NoopForwarder is a Forwarder doing nothing and not returning any responses.
type NoopForwarder struct{}

// Start does nothing.
func (f NoopForwarder) Start() error { return nil }

// Stop does nothing.
func (f NoopForwarder) Stop() {}

// SubmitV1Series does nothing.
func (f NoopForwarder) SubmitV1Series(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

// SubmitV1Intake does nothing.
func (f NoopForwarder) SubmitV1Intake(_ transaction.BytesPayloads, _ transaction.Kind, _ http.Header) error {
	return nil
}

// SubmitV1CheckRuns does nothing.
func (f NoopForwarder) SubmitV1CheckRuns(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

// SubmitSeries does nothing.
func (f NoopForwarder) SubmitSeries(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

// SubmitSketchSeries does nothing.
func (f NoopForwarder) SubmitSketchSeries(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

// SubmitHostMetadata does nothing.
func (f NoopForwarder) SubmitHostMetadata(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

// SubmitAgentChecksMetadata does nothing.
func (f NoopForwarder) SubmitAgentChecksMetadata(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

// SubmitMetadata does nothing.
func (f NoopForwarder) SubmitMetadata(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

// SubmitProcessChecks does nothing.
func (f NoopForwarder) SubmitProcessChecks(_ transaction.BytesPayloads, _ http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitProcessDiscoveryChecks does nothing.
func (f NoopForwarder) SubmitProcessDiscoveryChecks(_ transaction.BytesPayloads, _ http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitProcessEventChecks does nothing
func (f NoopForwarder) SubmitProcessEventChecks(_ transaction.BytesPayloads, _ http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitRTProcessChecks does nothing.
func (f NoopForwarder) SubmitRTProcessChecks(_ transaction.BytesPayloads, _ http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitContainerChecks does nothing.
func (f NoopForwarder) SubmitContainerChecks(_ transaction.BytesPayloads, _ http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitRTContainerChecks does nothing.
func (f NoopForwarder) SubmitRTContainerChecks(_ transaction.BytesPayloads, _ http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitConnectionChecks does nothing.
func (f NoopForwarder) SubmitConnectionChecks(_ transaction.BytesPayloads, _ http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitOrchestratorChecks does nothing.
func (f NoopForwarder) SubmitOrchestratorChecks(_ transaction.BytesPayloads, _ http.Header, _ int) error {
	return nil
}

// SubmitOrchestratorManifests does nothing.
func (f NoopForwarder) SubmitOrchestratorManifests(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}
