// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package defaultforwarderimpl

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

type testTransaction struct {
	mock.Mock
	assertClient bool
	processed    chan bool
	pointCount   int
	kind         transaction.Kind
	destination  transaction.Destination
	// shouldBlock causes Process to block on the worker's context, simulating
	// a request that aborts when its context is cancelled.
	shouldBlock bool
	// release, if non-nil, causes Process to block until the channel is
	// closed (or receives a value). Used to simulate an in-flight HTTP
	// request that hasn't yet returned.
	release chan struct{}
	Name    string
}

func newTestTransaction() *testTransaction {
	t := new(testTransaction)
	t.assertClient = true
	t.processed = make(chan bool, 1)
	return t
}

func newTestTransactionWithKind(kind transaction.Kind) *testTransaction {
	t := new(testTransaction)
	t.assertClient = true
	t.kind = kind
	t.processed = make(chan bool, 1)
	return t
}

func newTestTransactionWithoutClientAssert() *testTransaction {
	t := new(testTransaction)
	t.assertClient = false
	t.processed = make(chan bool, 1)
	return t
}

func newTestHTTPTransaction(domain string, pointCount int) *transaction.HTTPTransaction {
	t := transaction.NewHTTPTransaction()
	t.Domain = domain
	t.Endpoint = transaction.Endpoint{Route: "/api/v1/test", Name: "test-endpoint"}
	t.Payload = transaction.NewBytesPayload([]byte(`{"series":[]}`), pointCount)
	return t
}

func (t *testTransaction) GetCreatedAt() time.Time {
	return t.Called().Get(0).(time.Time)
}

func (t *testTransaction) Process(ctx context.Context, _ config.Component, _ log.Component, _ secrets.Component, client *http.Client, pointCountTelemetry transaction.PointCountTelemetry) error {
	defer func() { t.processed <- true }()

	var ret error

	// we always ignore the context to ease mocking
	if !t.assertClient {
		ret = t.Called().Error(0)
	} else {
		ret = t.Called(client).Error(0)
	}

	if t.shouldBlock {
		<-ctx.Done()
	}

	if t.release != nil {
		<-t.release
	}

	// Mirror HTTPTransaction.internalProcess: a nil-error outcome counts the
	// transaction's points as successfully sent. Tests of the worker rely on
	// this to assert points.sent accounting.
	if ret == nil && pointCountTelemetry != nil {
		pointCountTelemetry.OnPointSuccessfullySent(t.pointCount)
	}

	return ret
}

func (t *testTransaction) GetTarget() string {
	return t.Called().Get(0).(string)
}

func (t *testTransaction) GetPriority() transaction.Priority {
	return transaction.TransactionPriorityNormal
}

func (t *testTransaction) GetKind() transaction.Kind {
	return t.kind
}

func (t *testTransaction) GetDestination() transaction.Destination {
	return t.destination
}

func (t *testTransaction) GetEndpointName() string {
	return t.Name
}

func (t *testTransaction) GetPayloadSize() int {
	return t.Called().Get(0).(int)
}

func (t *testTransaction) SerializeTo(_ log.Component, _ transaction.TransactionsSerializer) error {
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
func (tf *MockedForwarder) SubmitV1Intake(payload transaction.BytesPayloads, _ transaction.Kind, extra http.Header) error {
	return tf.Called(payload, extra).Error(0)
}

// SubmitV1IntakeDirect updates the internal mock struct
func (tf *MockedForwarder) SubmitV1IntakeDirect(ctx context.Context, payload transaction.BytesPayloads, kind transaction.Kind, extra http.Header) error {
	return tf.Called(ctx, payload, kind, extra).Error(0)
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
func (tf *MockedForwarder) SubmitOrchestratorChecks(payload transaction.BytesPayloads, extra http.Header, _ int) error {
	return tf.Called(payload, extra).Error(0)
}

// SubmitOrchestratorManifests mock
func (tf *MockedForwarder) SubmitOrchestratorManifests(payload transaction.BytesPayloads, extra http.Header) error {
	return tf.Called(payload, extra).Error(0)
}

// GetDomainResolvers returns the list of resolvers used by this forwarder.
func (tf *MockedForwarder) GetDomainResolvers() []resolver.DomainResolver {
	return tf.Called().Get(0).([]resolver.DomainResolver)
}

// SubmitTransaction adds a transaction to the queue for sending.
func (tf *MockedForwarder) SubmitTransaction(t *transaction.HTTPTransaction) error {
	return tf.Called(t).Error(0)
}

type handlerTransport http.HandlerFunc

func (tr handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	tr(rec, req)
	return rec.Result(), nil
}

// pipeAddr is a placeholder net.Addr for pipeListener, which has no real
// network address.
type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

// pipeListener is a net.Listener backed by in-memory net.Pipe connections.
// Tests that run an http.Server inside a testing/synctest bubble use it
// instead of a real TCP listener (e.g. httptest.NewServer): a goroutine
// blocked reading or writing a real socket is never "durably blocked" (see
// the testing/synctest docs), which would prevent the bubble from ever going
// idle. net.Pipe, by contrast, is implemented entirely with plain channels,
// so blocking reads/writes on it are recognized by synctest and the bubble
// can still make progress deterministically.
type pipeListener struct {
	conns  chan net.Conn
	closed chan struct{}
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		conns:  make(chan net.Conn),
		closed: make(chan struct{}),
	}
}

// dial creates a new in-memory connection pair, hands the server half to a
// pending (or future) Accept call, and returns the client half. It is meant
// to be used as an http.Transport's DialContext/DialTLSContext dependency.
func (l *pipeListener) dial(_ context.Context, _, _ string) (net.Conn, error) {
	serverConn, clientConn := net.Pipe()
	select {
	case l.conns <- serverConn:
		return clientConn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

// NewTestForwarder creates an instance of the component based on config, but without using fx or starting it.
func NewTestForwarder(params defaultforwarderdef.Params, config config.Component, log log.Component, secrets secrets.Component) (Forwarder, error) {
	opts, err := createOptions(params, config, log, secrets)
	if err != nil {
		return nil, err
	}
	return NewDefaultForwarder(config, log, opts), nil
}
