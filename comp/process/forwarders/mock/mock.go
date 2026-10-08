// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package forwardersmock provides a mock for the forwarders component.
package forwardersmock

import (
	"net/http"
	"testing"

	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwardernoop "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/noop-impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	forwarders "github.com/DataDog/datadog-agent/comp/process/forwarders/def"
)

// Mock implements the forwarders component with no-op forwarders.
type Mock struct {
	processForwarder     defaultforwarder.Component
	rtProcessForwarder   defaultforwarder.Component
	connectionsForwarder connectionsforwarder.Component
}

// New returns a mock forwarders component.
func New(_ testing.TB) forwarders.Component {
	return &Mock{
		processForwarder:     noopForwarder{defaultforwardernoop.NewComponent()},
		rtProcessForwarder:   noopForwarder{defaultforwardernoop.NewComponent()},
		connectionsForwarder: noopForwarder{defaultforwardernoop.NewComponent()},
	}
}

// GetProcessForwarder returns the no-op process forwarder.
func (m *Mock) GetProcessForwarder() defaultforwarder.Component {
	return m.processForwarder
}

// GetRTProcessForwarder returns the no-op real-time process forwarder.
func (m *Mock) GetRTProcessForwarder() defaultforwarder.Component {
	return m.rtProcessForwarder
}

// GetConnectionsForwarder returns the no-op connections forwarder.
func (m *Mock) GetConnectionsForwarder() connectionsforwarder.Component {
	return m.connectionsForwarder
}

type noopForwarder struct {
	defaultforwarder.Component
}

func closedResponses() (chan defaultforwarder.Response, error) {
	responses := make(chan defaultforwarder.Response)
	close(responses)
	return responses, nil
}

func (noopForwarder) SubmitProcessChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return closedResponses()
}

func (noopForwarder) SubmitProcessDiscoveryChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return closedResponses()
}

func (noopForwarder) SubmitRTProcessChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return closedResponses()
}

func (noopForwarder) SubmitContainerChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return closedResponses()
}

func (noopForwarder) SubmitRTContainerChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return closedResponses()
}

func (noopForwarder) SubmitConnectionChecks(transaction.BytesPayloads, http.Header) (chan defaultforwarder.Response, error) {
	return closedResponses()
}
