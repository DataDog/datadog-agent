// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package defaultforwarder

import (
	"context"
	"net/http"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

type testTransaction struct {
	mock.Mock
	assertClient bool
	processed    chan bool
	pointCount   int
}

func newTestTransaction() *testTransaction {
	t := new(testTransaction)
	t.assertClient = true
	t.processed = make(chan bool, 1)
	return t
}

func newTestTransactionWithoutClientAssert() *testTransaction {
	t := new(testTransaction)
	t.assertClient = false
	t.processed = make(chan bool, 1)
	return t
}

func (t *testTransaction) GetCreatedAt() time.Time {
	return t.Called().Get(0).(time.Time)
}

func (t *testTransaction) Process(_ context.Context, _ config.Component, client *http.Client) error {
	defer func() { t.processed <- true }()
	// we always ignore the context to ease mocking
	if !t.assertClient {
		return t.Called().Error(0)
	}
	return t.Called(client).Error(0)
}

func (t *testTransaction) GetTarget() string {
	return t.Called().Get(0).(string)
}

func (t *testTransaction) GetPriority() transaction.Priority {
	return transaction.TransactionPriorityNormal
}

func (t *testTransaction) GetEndpointName() string {
	return ""
}

func (t *testTransaction) GetPayloadSize() int {
	return t.Called().Get(0).(int)
}

func (t *testTransaction) SerializeTo(serializer transaction.TransactionsSerializer) error {
	return nil
}

func (t *testTransaction) GetPointCount() int {
	return t.pointCount
}

// Compile-time checking to ensure that MockedForwarder implements Forwarder
var _ Forwarder = &MockedForwarder{}

// MockedForwarder a mocked forwarder to be use in other module to test their dependencies with the forwarder
type MockedForwarder struct {
	mock.Mock
}

// Start updates the internal mock struct
func (tf *MockedForwarder) Start() error {
	return tf.Called().Error(0)
}

// Stop updates the internal mock struct
func (tf *MockedForwarder) Stop() {
	tf.Called()
}

// SubmitV1Series updates the internal mock struct
func (tf *MockedForwarder) SubmitV1Series(payload transaction.BytesPayloads, extra http.Header) error {
	return tf.Called(payload, extra).Error(0)
}

// SubmitSeries updates the internal mock struct
func (tf *MockedForwarder) SubmitSeries(payload transaction.BytesPayloads, extra http.Header) error {
	return tf.Called(payload, extra).Error(0)
}

// SubmitV1Intake updates the internal mock struct
func (tf *MockedForwarder) SubmitV1Intake(payload transaction.BytesPayloads, extra http.Header) error {
	return tf.Called(payload, extra).Error(0)
}

// SubmitV1CheckRuns updates the internal mock struct
func (tf *MockedForwarder) SubmitV1CheckRuns(payload transaction.BytesPayloads, extra http.Header) error {
	return tf.Called(payload, extra).Error(0)
}

// SubmitSketchSeries updates the internal mock struct
func (tf *MockedForwarder) SubmitSketchSeries(payload transaction.BytesPayloads, extra http.Header) error {
	return tf.Called(payload, extra).Error(0)
}

// SubmitHostMetadata updates the internal mock struct
func (tf *MockedForwarder) SubmitHostMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return tf.Called(payload, extra).Error(0)
}

// SubmitAgentChecksMetadata updates the internal mock struct
func (tf *MockedForwarder) SubmitAgentChecksMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return tf.Called(payload, extra).Error(0)
}

// SubmitMetadata updates the internal mock struct
func (tf *MockedForwarder) SubmitMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return tf.Called(payload, extra).Error(0)
}

// SubmitProcessChecks mock
func (tf *MockedForwarder) SubmitProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return nil, tf.Called(payload, extra).Error(0)
}

// SubmitProcessDiscoveryChecks mock
func (tf *MockedForwarder) SubmitProcessDiscoveryChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return nil, tf.Called(payload, extra).Error(0)
}

// SubmitProcessEventChecks mock
func (tf *MockedForwarder) SubmitProcessEventChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return nil, tf.Called(payload, extra).Error(0)
}

// SubmitRTProcessChecks mock
func (tf *MockedForwarder) SubmitRTProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return nil, tf.Called(payload, extra).Error(0)
}

// SubmitContainerChecks mock
func (tf *MockedForwarder) SubmitContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return nil, tf.Called(payload, extra).Error(0)
}

// SubmitRTContainerChecks mock
func (tf *MockedForwarder) SubmitRTContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return nil, tf.Called(payload, extra).Error(0)
}

// SubmitConnectionChecks mock
func (tf *MockedForwarder) SubmitConnectionChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return nil, tf.Called(payload, extra).Error(0)
}

// SubmitOrchestratorChecks mock
func (tf *MockedForwarder) SubmitOrchestratorChecks(payload transaction.BytesPayloads, extra http.Header, payloadType int) (chan Response, error) {
	return nil, tf.Called(payload, extra).Error(0)
}

// SubmitOrchestratorManifests mock
func (tf *MockedForwarder) SubmitOrchestratorManifests(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return nil, tf.Called(payload, extra).Error(0)
}
