// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarder

import "net/http"

// NoopForwarder is a Forwarder doing nothing and not returning any responses.
type NoopForwarder struct{}

// Start does nothing.
func (f NoopForwarder) Start() error { return nil }

// Stop does nothing.
func (f NoopForwarder) Stop() {}

// SubmitV1Series does nothing.
func (f NoopForwarder) SubmitV1Series(payload Payloads, extra http.Header) error { return nil }

// SubmitV1Intake does nothing.
func (f NoopForwarder) SubmitV1Intake(payload Payloads, extra http.Header) error { return nil }

// SubmitV1CheckRuns does nothing.
func (f NoopForwarder) SubmitV1CheckRuns(payload Payloads, extra http.Header) error { return nil }

// SubmitSeries does nothing.
func (f NoopForwarder) SubmitSeries(payload Payloads, extra http.Header) error { return nil }

// SubmitSketchSeries does nothing.
func (f NoopForwarder) SubmitSketchSeries(payload Payloads, extra http.Header) error { return nil }

// SubmitHostMetadata does nothing.
func (f NoopForwarder) SubmitHostMetadata(payload Payloads, extra http.Header) error { return nil }

// SubmitAgentChecksMetadata does nothing.
func (f NoopForwarder) SubmitAgentChecksMetadata(payload Payloads, extra http.Header) error {
	return nil
}

// SubmitMetadata does nothing.
func (f NoopForwarder) SubmitMetadata(payload Payloads, extra http.Header) error { return nil }

// SubmitProcessChecks does nothing.
func (f NoopForwarder) SubmitProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitProcessDiscoveryChecks does nothing.
func (f NoopForwarder) SubmitProcessDiscoveryChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitProcessEventChecks does nothing
func (f NoopForwarder) SubmitProcessEventChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitRTProcessChecks does nothing.
func (f NoopForwarder) SubmitRTProcessChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitContainerChecks does nothing.
func (f NoopForwarder) SubmitContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitRTContainerChecks does nothing.
func (f NoopForwarder) SubmitRTContainerChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitConnectionChecks does nothing.
func (f NoopForwarder) SubmitConnectionChecks(payload Payloads, extra http.Header) (chan Response, error) {
	return nil, nil
}

// SubmitOrchestratorChecks does nothing.
func (f NoopForwarder) SubmitOrchestratorChecks(payload Payloads, extra http.Header, payloadType int) (chan Response, error) {
	return nil, nil
}

// SubmitContainerLifecycleEvents does nothing.
func (f NoopForwarder) SubmitContainerLifecycleEvents(payload Payloads, extra http.Header) error {
	return nil
}
