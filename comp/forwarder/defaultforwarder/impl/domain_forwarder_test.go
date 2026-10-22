// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package defaultforwarderimpl

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/internal/retry"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	mock "github.com/DataDog/datadog-agent/pkg/config/mock"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

func TestNewDomainForwarder(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 120*time.Second, false)

	assert.NotNil(t, forwarder)
	assert.Equal(t, 1, forwarder.concurrency.numWorkers())
	assert.Equal(t, 120*time.Second, forwarder.connectionResetInterval)
	assert.Equal(t, Stopped, forwarder.State())
	assert.Nil(t, forwarder.highPrio)
	assert.Nil(t, forwarder.lowPrio)
	assert.Nil(t, forwarder.requeuedTransaction)
	assert.Nil(t, forwarder.stopRetry)
	assert.Nil(t, forwarder.stopConnectionReset)
	assert.Len(t, forwarder.workers, 0)
	requireLenForwarderRetryQueue(t, forwarder, 0)
	assert.NotNil(t, forwarder.blockedList, 0)
}

func TestDomainForwarderStart(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, false)
	err := forwarder.Start()

	assert.NoError(t, err)
	requireLenForwarderRetryQueue(t, forwarder, 0)
	require.Len(t, forwarder.workers, 1)
	assert.Equal(t, Started, forwarder.State())
	assert.NotNil(t, forwarder.highPrio)
	assert.NotNil(t, forwarder.lowPrio)
	assert.NotNil(t, forwarder.requeuedTransaction)
	assert.NotNil(t, forwarder.stopRetry)
	assert.NotNil(t, forwarder.stopConnectionReset)

	assert.NotNil(t, forwarder.Start())

	forwarder.Stop(false)
}

func TestDomainForwarderSemaphorePerPolicy(t *testing.T) {
	newForwarder := func(t *testing.T, concurrency concurrencyDomain) *domainForwarder {
		mockConfig := mock.New(t)
		log := logmock.New(t)
		retryQueue := retry.NewTransactionRetryQueue(nil, 2, 0, retry.NewTransactionRetryQueueTelemetry("domain"), retry.NewPointCountTelemetryMock())
		return newDomainForwarder(mockConfig, log, secretsmock.New(t), "test", false, false, retryQueue, concurrency, 0, retry.NewPointCountTelemetry("domain"), nil)
	}

	t.Run("static gives each worker its own semaphore", func(t *testing.T) {
		forwarder := newForwarder(t, newTestConcurrencyDomain(3, 1))
		require.NoError(t, forwarder.Start())
		defer forwarder.Stop(false)

		require.Len(t, forwarder.workers, 3)
		distinct := map[weightedSemaphore]struct{}{}
		for _, w := range forwarder.workers {
			distinct[w.maxConcurrentRequests] = struct{}{}
		}
		assert.Len(t, distinct, 3)
		assert.IsType(t, &http.Transport{}, forwarder.Client.GetClient().Transport)
	})

}

func TestDomainForwarderInit(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, false)
	forwarder.init()
	assert.Len(t, forwarder.workers, 0)
	requireLenForwarderRetryQueue(t, forwarder, 0)
}

func TestDomainForwarderStop(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, false)
	forwarder.Stop(false) // this should be a noop
	forwarder.Start()
	assert.Equal(t, Started, forwarder.State())
	forwarder.Stop(false)
	assert.Len(t, forwarder.workers, 0)
	requireLenForwarderRetryQueue(t, forwarder, 0)
	assert.Equal(t, Stopped, forwarder.State())
}

func TestDomainForwarderStop_WithConnectionReset(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 120*time.Second, false)
	forwarder.Stop(false) // this should be a noop
	forwarder.Start()
	assert.Equal(t, Started, forwarder.State())
	forwarder.Stop(false)
	assert.Len(t, forwarder.workers, 0)
	requireLenForwarderRetryQueue(t, forwarder, 0)
	assert.Equal(t, Stopped, forwarder.State())
}

func TestDomainForwarderSendHTTPTransactions(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, false)
	tr := newTestTransactionDomainForwarder()

	// fw is stopped, we should get an error
	forwarder.sendHTTPTransactions(tr)

	defer forwarder.Stop(false)
	forwarder.Start()
	// Stopping the worker for the TestRequeueTransaction
	forwarder.workers[0].Stop(false)

	forwarder.sendHTTPTransactions(tr)
	transactionToProcess := <-forwarder.highPrio
	assert.Equal(t, tr, transactionToProcess)

	// Reset `forwarder.workers` otherwise `defer forwarder.Stop(false)` will timeout.
	forwarder.workers = nil
}

func TestDomainForwarderHAPreFailover(t *testing.T) {
	synctest.Test(t, syncTestDomainForwarderHAPreFailover)
}

func syncTestDomainForwarderHAPreFailover(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("multi_region_failover.enabled", true)
	mockConfig.SetInTest("multi_region_failover.failover_metrics", false)
	mockConfig.SetInTest("multi_region_failover.apikey", "foo")
	mockConfig.SetInTest("multi_region_failover.site", "bar.ddhq.com")

	log := logmock.New(t)
	// HA forwarder
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, true)
	trSeries := newTestTransactionWithKindDomainForwarder(transaction.Series)
	trMeta := newTestTransactionWithKindDomainForwarder(transaction.Metadata)

	// fw is stopped, we should get an error
	forwarder.sendHTTPTransactions(trMeta)
	forwarder.sendHTTPTransactions(trSeries)

	defer forwarder.Stop(false)
	forwarder.Start()

	// Stopping the worker
	forwarder.workers[0].Stop(false)

	trSeries.On("Process", forwarder.workers[0].Client).Return(nil).Times(1)
	trMeta.On("Process", forwarder.workers[0].Client).Return(nil).Times(1)

	forwarder.sendHTTPTransactions(trMeta)
	transactionToProcess := <-forwarder.highPrio
	assert.Equal(t, trMeta, transactionToProcess)

	// haven't failed over, so transaction should be dropped and not queued
	forwarder.sendHTTPTransactions(trSeries)
	timedOut := false
	select {
	case <-forwarder.highPrio:
		// should not happen
	case <-time.After(time.Second):
		timedOut = true
	}
	assert.Equal(t, timedOut, true)

	// Reset `forwarder.workers` otherwise `defer forwarder.Stop(false)` will timeout.
	forwarder.workers = nil
}

func TestDomainForwarderHAFailover(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("multi_region_failover.enabled", true)
	mockConfig.SetInTest("multi_region_failover.failover_metrics", true)
	mockConfig.SetInTest("multi_region_failover.apikey", "foo")
	mockConfig.SetInTest("multi_region_failover.site", "bar.ddhq.com")

	log := logmock.New(t)
	// HA forwarder
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, true)
	trSeries := newTestTransactionWithKindDomainForwarder(transaction.Series)
	trMeta := newTestTransactionWithKindDomainForwarder(transaction.Metadata)

	// fw is stopped, we should get an error
	forwarder.sendHTTPTransactions(trMeta)
	forwarder.sendHTTPTransactions(trSeries)

	defer forwarder.Stop(false)
	forwarder.Start()

	// Stopping the worker
	forwarder.workers[0].Stop(false)

	trSeries.On("Process", forwarder.workers[0].Client).Return(nil).Times(1)
	trMeta.On("Process", forwarder.workers[0].Client).Return(nil).Times(1)

	forwarder.sendHTTPTransactions(trMeta)
	transactionToProcess := <-forwarder.highPrio
	assert.Equal(t, trMeta, transactionToProcess)

	// haven't failed over, so transaction should be dropped and not queued
	forwarder.sendHTTPTransactions(trSeries)
	timedOut := false
	select {
	case transactionToProcess = <-forwarder.highPrio:
		// should not happen
	case <-time.After(time.Second):
		timedOut = true
	}
	assert.Equal(t, timedOut, false)
	assert.Equal(t, trSeries, transactionToProcess)

	// Reset `forwarder.workers` otherwise `defer forwarder.Stop(false)` will timeout.
	forwarder.workers = nil
}

func TestRequeueTransaction(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, false)
	tr := transaction.NewHTTPTransaction()
	requireLenForwarderRetryQueue(t, forwarder, 0)
	forwarder.requeueTransaction(tr)
	requireLenForwarderRetryQueue(t, forwarder, 1)
}

func TestRetryTransactions(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, false)
	forwarder.init()

	// Default value should be 0
	assert.Equal(t, int64(0), transaction.TransactionsDropped.Value())

	payload := transaction.NewBytesPayloadWithoutMetaData([]byte{1})
	t1 := transaction.NewHTTPTransaction()
	t1.Domain = "domain/"
	t1.Endpoint.Name = "test1"
	t1.Endpoint.Route = "test1"
	t1.Payload = payload
	t2 := transaction.NewHTTPTransaction()
	t2.Domain = "domain/"
	t2.Endpoint.Name = "test2"
	t2.Endpoint.Route = "test2"
	t2.Payload = payload

	// Create blocks
	forwarder.blockedList.recover(t1.GetTarget(), time.Now())
	forwarder.blockedList.recover(t2.GetTarget(), time.Now())

	forwarder.blockedList.errorPerEndpoint[t1.GetTarget()].until = time.Now().Add(-1 * time.Hour)
	forwarder.blockedList.errorPerEndpoint[t2.GetTarget()].until = time.Now().Add(1 * time.Hour)
	forwarder.blockedList.errorPerEndpoint[t2.GetTarget()].state = blocked

	forwarder.requeueTransaction(t2)
	forwarder.requeueTransaction(t2) // this second one should be dropped
	forwarder.requeueTransaction(t1) // the queue should be sorted
	forwarder.retryTransactions(time.Now())
	requireLenForwarderRetryQueue(t, forwarder, 1)
	assert.Len(t, forwarder.lowPrio, 1)
	assert.Equal(t, int64(1), transaction.TransactionsDropped.Value())
}

func TestForwarderRetry(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, false)
	forwarder.Start()
	defer forwarder.Stop(false)

	forwarder.blockedList.close("blocked", time.Now())
	forwarder.blockedList.errorPerEndpoint["blocked"].until = time.Now().Add(1 * time.Hour)
	forwarder.blockedList.errorPerEndpoint["blocked"].state = blocked

	ready := newTestTransactionDomainForwarder()
	notReady := newTestTransactionDomainForwarder()

	forwarder.requeueTransaction(ready)
	forwarder.requeueTransaction(notReady)
	requireLenForwarderRetryQueue(t, forwarder, 2)

	ready.On("Process", forwarder.workers[0].Client.GetClient()).Return(nil).Times(1)
	ready.On("GetTarget").Return("").Times(2)
	ready.On("GetCreatedAt").Return(time.Now()).Times(1)
	notReady.On("GetCreatedAt").Return(time.Now()).Times(1)
	notReady.On("GetTarget").Return("blocked").Times(1)

	forwarder.retryTransactions(time.Now())
	<-ready.processed

	ready.AssertExpectations(t)
	notReady.AssertExpectations(t)
	notReady.AssertNumberOfCalls(t, "Process", 0)
	notReady.AssertNumberOfCalls(t, "GetTarget", 1)
	trs, err := forwarder.retryQueue.ExtractTransactions()
	require.NoError(t, err)
	require.Len(t, trs, 1)
	assert.Equal(t, trs[0], notReady)
}

func TestForwarderRetryLifo(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, false)
	forwarder.init()

	transaction1 := newTestTransactionDomainForwarder()
	transaction2 := newTestTransactionDomainForwarder()

	forwarder.requeueTransaction(transaction1)
	forwarder.requeueTransaction(transaction2)

	transaction1.On("GetCreatedAt").Return(time.Now()).Times(1)
	transaction1.On("GetTarget").Return("").Times(1)

	transaction2.On("GetCreatedAt").Return(time.Now().Add(1 * time.Minute)).Times(1)
	transaction2.On("GetTarget").Return("").Times(1)

	forwarder.retryTransactions(time.Now())

	firstOut := <-forwarder.lowPrio
	assert.Equal(t, firstOut, transaction2)

	secondOut := <-forwarder.lowPrio
	assert.Equal(t, secondOut, transaction1)

	transaction1.AssertExpectations(t)
	transaction2.AssertExpectations(t)
	requireLenForwarderRetryQueue(t, forwarder, 0)
}

func TestForwarderRetryLimitQueue(t *testing.T) {
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, false)
	forwarder.init()
	forwarder.blockedList.close("blocked", time.Now())
	forwarder.blockedList.errorPerEndpoint["blocked"].until = time.Now().Add(1 * time.Minute)

	var transactions []*testTransaction
	for _, v := range []time.Time{time.Now(), time.Now().Add(1 * time.Minute), time.Now().Add(2 * time.Minute)} {
		transaction := newTestTransactionDomainForwarder()

		forwarder.requeueTransaction(transaction)
		transaction.On("GetCreatedAt").Return(v).Maybe()
		transaction.On("GetTarget").Return("blocked").Maybe()
		transactions = append(transactions, transaction)
	}

	forwarder.retryTransactions(time.Now())
	for _, tr := range transactions {
		tr.AssertExpectations(t)
	}

	require.Len(t, forwarder.highPrio, 0)
	require.Len(t, forwarder.lowPrio, 0)
	trs, err := forwarder.retryQueue.ExtractTransactions()
	require.NoError(t, err)
	require.Len(t, trs, 2)

	// assert that the oldest transaction was dropped (transactions[0] has time=Now)
	assert.Equal(t, transactions[2], trs[0])
	assert.Equal(t, transactions[1], trs[1])
}

func TestDomainForwarderRetryQueueAllPayloadsMaxSize(t *testing.T) {
	oldFlushInterval := flushInterval
	defer func() { flushInterval = oldFlushInterval }()
	flushInterval = 1 * time.Minute

	telemetry := retry.NewTransactionRetryQueueTelemetry("domain")
	transactionRetryQueue := retry.NewTransactionRetryQueue(
		nil,
		1+2,
		0,
		telemetry,
		retry.NewPointCountTelemetryMock())
	mockConfig := mock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	forwarder := newDomainForwarder(mockConfig, log, secrets, "test", false, false, transactionRetryQueue, newTestConcurrencyDomain(0, 1), 10, retry.NewPointCountTelemetry("domain"), nil)
	forwarder.blockedList.close("blocked", time.Now())
	forwarder.blockedList.errorPerEndpoint["blocked"].until = time.Now().Add(1 * time.Minute)

	defer forwarder.Stop(true)
	forwarder.Start()

	for _, payloadSize := range []int{4, 3, 2, 1} {
		tr := newTestTransaction()
		tr.On("GetPayloadSize").Return(payloadSize)
		tr.On("GetTarget").Return("blocked")
		tr.On("GetCreatedAt").Return(time.Now().Add(time.Duration(-payloadSize) * time.Second))
		transactionRetryQueue.Add(tr)
	}

	forwarder.retryTransactions(time.Now())

	trs, err := transactionRetryQueue.ExtractTransactions()
	require.NoError(t, err)
	require.Len(t, trs, 2)
	require.Equal(t, 1, trs[0].GetPayloadSize())
	require.Equal(t, 2, trs[1].GetPayloadSize())
}

func TestDomainForwarderInitConfigs(t *testing.T) {
	// Test default values
	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, false)
	forwarder.init()
	assert.Equal(t, 100, cap(forwarder.highPrio))
	assert.Equal(t, 100, cap(forwarder.lowPrio))
	assert.Equal(t, 100, cap(forwarder.requeuedTransaction))

	// Test custom values
	datadogYaml := `
forwarder_high_prio_buffer_size: 1100
forwarder_low_prio_buffer_size: 1200
forwarder_requeue_buffer_size: 1300
`
	mockConfig = mock.NewFromYAML(t, datadogYaml)

	forwarder = newDomainForwarderForTest(t, mockConfig, log, 0, false)
	forwarder.init()
	assert.Equal(t, 1100, cap(forwarder.highPrio))
	assert.Equal(t, 1200, cap(forwarder.lowPrio))
	assert.Equal(t, 1300, cap(forwarder.requeuedTransaction))
}

// mockDiskStorage is a TransactionDiskStorage implementation used in tests that
// captures every transaction passed to Store without performing any I/O.
type mockDiskStorage struct {
	allStored []transaction.Transaction
}

func (m *mockDiskStorage) Store(ts []transaction.Transaction) error {
	m.allStored = append(m.allStored, ts...)
	return nil
}

func (m *mockDiskStorage) ExtractLast() ([]transaction.Transaction, error) { return nil, nil }

func (m *mockDiskStorage) GetDiskSpaceUsed() int64 { return 0 }

// TestDomainForwarderStopPreservesLowPrioTransactions verifies that transactions
// present in the lowPrio channel at shutdown time are transferred to the retry
// queue rather than silently dropped.
func TestDomainForwarderStopPreservesLowPrioTransactions(t *testing.T) {
	oldFlushInterval := flushInterval
	defer func() { flushInterval = oldFlushInterval }()
	flushInterval = time.Hour

	mockConfig := mock.New(t)
	log := logmock.New(t)
	forwarder := newDomainForwarderForTest(t, mockConfig, log, 0, false)
	forwarder.Start()

	// Stop the single worker so it cannot consume from lowPrio before Stop drains it.
	forwarder.workers[0].Stop(false)
	forwarder.workers = nil // Prevent Stop from re-stopping the already-stopped worker.

	tr := newTestTransactionDomainForwarder()
	forwarder.lowPrio <- tr

	forwarder.Stop(false)

	requireLenForwarderRetryQueue(t, forwarder, 1)
}

// TestDomainForwarderStopFlushesRetryQueueAndLowPrioToDisk verifies that at
// shutdown, transactions held in both the in-memory retry queue and the lowPrio
// channel are all written to disk via FlushToDisk.
func TestDomainForwarderStopFlushesRetryQueueAndLowPrioToDisk(t *testing.T) {
	oldFlushInterval := flushInterval
	defer func() { flushInterval = oldFlushInterval }()
	flushInterval = time.Hour

	storage := &mockDiskStorage{}
	telemetry := retry.NewTransactionRetryQueueTelemetry("domain")
	retryQueue := retry.NewTransactionRetryQueue(
		storage,
		100,
		0,
		telemetry,
		retry.NewPointCountTelemetryMock())

	mockConfig := mock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	forwarder := newDomainForwarder(mockConfig, log, secrets, "test", false, false, retryQueue, newTestConcurrencyDomain(1, 1), 0, retry.NewPointCountTelemetry("domain"), nil)
	forwarder.Start()

	forwarder.workers[0].Stop(false)
	forwarder.workers = nil

	tr1 := newTestTransactionDomainForwarder()
	tr1.On("GetCreatedAt").Return(time.Now()).Maybe()
	forwarder.requeueTransaction(tr1)

	tr2 := newTestTransactionDomainForwarder()
	tr2.On("GetCreatedAt").Return(time.Now()).Maybe()
	forwarder.lowPrio <- tr2

	forwarder.Stop(false)

	hasTx := func(target transaction.Transaction) bool {
		for _, tx := range storage.allStored {
			if tx == target {
				return true
			}
		}
		return false
	}
	require.Len(t, storage.allStored, 2)
	assert.True(t, hasTx(tr1), "transaction from retry queue should be flushed to disk")
	assert.True(t, hasTx(tr2), "transaction from lowPrio should be flushed to disk")
}

func newDomainForwarderForTest(t *testing.T, config config.Component, log log.Component, connectionResetInterval time.Duration, ha bool) *domainForwarder {
	telemetry := retry.NewTransactionRetryQueueTelemetry("domain")
	transactionRetryQueue := retry.NewTransactionRetryQueue(
		nil,
		2,
		0,
		telemetry,
		retry.NewPointCountTelemetryMock())

	secrets := secretsmock.New(t)
	return newDomainForwarder(config, log, secrets, "test", ha, false, transactionRetryQueue, newTestConcurrencyDomain(1, 1), connectionResetInterval, retry.NewPointCountTelemetry("domain"), nil)
}

func requireLenForwarderRetryQueue(t *testing.T, forwarder *domainForwarder, expectedValue int) {
	require.Equal(t, expectedValue, forwarder.retryQueue.GetTransactionCount())
}

func newTestTransactionDomainForwarder() *testTransaction {
	tr := newTestTransaction()
	tr.On("GetPayloadSize").Return(1)
	return tr
}

func newTestTransactionWithKindDomainForwarder(kind transaction.Kind) *testTransaction {
	tr := newTestTransactionWithKind(kind)
	tr.On("GetPayloadSize").Return(1)
	tr.On("GetTarget").Return("foo.ddhq.com")
	return tr
}

// burstTx is a transaction whose Process blocks until release is closed. It
// stands in for a request to a slow or unreachable intake.
type burstTx struct {
	transaction.Transaction // nil: a call we did not override panics
	release                 chan struct{}
	started                 *atomic.Int32
}

func (b *burstTx) Process(_ context.Context, _ config.Component, _ log.Component, _ secrets.Component, _ *http.Client, _ transaction.PointCountTelemetry) error {
	b.started.Add(1)
	<-b.release
	return nil
}
func (b *burstTx) GetCreatedAt() time.Time           { return time.Now() }
func (b *burstTx) GetTarget() string                 { return "burst.test" }
func (b *burstTx) GetPriority() transaction.Priority { return transaction.TransactionPriorityNormal }
func (b *burstTx) GetEndpointName() string           { return "series_v2" }
func (b *burstTx) GetPayloadSize() int               { return 1000 }
func (b *burstTx) GetPointCount() int                { return 10 }

// TestForwarderBurstAbsorptionCliff pins how many payloads a slow intake lets the
// forwarder hold with default buffers: 100 (channel) + 10 (in flight) + 1 (dispatcher
// blocked on the semaphore) = 111. The rest go to the retry queue. When it is full,
// each new payload evicts the oldest and counts as a drop. The test waits until the
// dispatcher has taken its 11, so the count is exact. In production the serializer
// sends payloads in a tight loop, so the channel can fill first and the real limit
// sits between 100 and 111.
func TestForwarderBurstAbsorptionCliff(t *testing.T) {
	const retrySlots = 39 // like 15 MB / 0.4 MB per payload
	for _, tc := range []struct {
		payloads, queued int
		dropped          int64
	}{
		{50, 0, 0},
		{111, 0, 0},
		{111 + retrySlots, retrySlots, 0},
		{111 + retrySlots + 1, retrySlots, 1},
		{111 + retrySlots + 50, retrySlots, 50},
	} {
		t.Run(fmt.Sprintf("%d_payloads", tc.payloads), func(t *testing.T) {
			cfg := mock.New(t)
			log := logmock.New(t)
			require.Equal(t, 100, cfg.GetInt("forwarder_high_prio_buffer_size"))
			require.Equal(t, 10, cfg.GetInt("forwarder_max_concurrent_requests"))
			retryQueue := retry.NewTransactionRetryQueue(nil, retrySlots*1000, 0,
				retry.NewTransactionRetryQueueTelemetry("burst"), retry.NewPointCountTelemetryMock())
			f := newDomainForwarder(cfg, log, secretsmock.New(t), "burst.test", false, false,
				retryQueue, concurrencyPolicyFromConfig(cfg, log).newDomain("burst.test"), 0,
				retry.NewPointCountTelemetry("burst"), nil)
			require.NoError(t, f.Start())
			tx := &burstTx{release: make(chan struct{}), started: &atomic.Int32{}}
			t.Cleanup(func() { close(tx.release); f.Stop(false) })
			droppedBefore := transaction.TransactionsDropped.Value()

			// Wait for the dispatcher to take its 11 payloads, then send the rest.
			for i := 0; i < tc.payloads; i++ {
				f.sendHTTPTransactions(tx)
				if i == 10 {
					require.Eventually(t, func() bool { return tx.started.Load() == 10 && len(f.highPrio) == 0 },
						5*time.Second, time.Millisecond)
				}
			}

			assert.Equal(t, tc.queued, f.retryQueue.GetTransactionCount(), "retry queue depth")
			assert.Equal(t, tc.dropped, transaction.TransactionsDropped.Value()-droppedBefore, "transactions dropped")
		})
	}
}

// TestHTTP1MaxConnections checks how many connections per domain the forwarder
// client opens over HTTP/1 and that it keeps them idle between bursts instead of
// closing and redialing them. Each burst sends forwarder_max_concurrent_requests
// requests at once. The server holds them until the expected number is in flight,
// waits a little longer to catch extra ones, then releases them.
func TestHTTP1MaxConnections(t *testing.T) {
	for _, tc := range []struct {
		name           string
		protocol       string
		maxConnections int
		want           int
	}{
		{"http1 default keeps one connection", "http1", 0, 1},
		{"http1 opt-in 4", "http1", 4, 4},
		{"http1 opt-in 8, above the default idle pool", "http1", 8, 8},
		{"auto ignores it", "auto", 4, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mock.New(t)
			log := logmock.New(t)
			cfg.SetInTest("forwarder_http_protocol", tc.protocol)
			if tc.maxConnections != 0 {
				cfg.SetInTest("forwarder_http1_max_connections", tc.maxConnections)
			}
			requests := cfg.GetInt("forwarder_max_concurrent_requests")
			require.Equal(t, 10, requests)
			require.Equal(t, 1, cfg.GetInt("forwarder_num_workers"))

			maxConns := concurrencyPolicyFromConfig(cfg, log).maxConns()
			require.Equal(t, tc.want, maxConns)
			defaults := httputils.CreateHTTPTransport(cfg)
			transport := NewHTTPTransport(cfg, maxConns, log)
			assert.Equal(t, tc.want, transport.MaxConnsPerHost)
			assert.Equal(t, max(defaults.MaxIdleConnsPerHost, tc.want), transport.MaxIdleConnsPerHost, "idle pool per host")
			assert.Equal(t, defaults.MaxIdleConns, transport.MaxIdleConns, "idle pool")

			type gate struct {
				allIn, release chan struct{}
				once           sync.Once
			}
			var mu sync.Mutex
			var cur *gate
			var inflight, peak int
			var newConns atomic.Int32

			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				g := cur
				inflight++
				peak = max(peak, inflight)
				if inflight == tc.want {
					g.once.Do(func() { close(g.allIn) })
				}
				mu.Unlock()
				<-g.release
				mu.Lock()
				inflight--
				mu.Unlock()
				w.WriteHeader(http.StatusAccepted)
			}))
			srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					newConns.Add(1)
				}
			}
			srv.Start()
			t.Cleanup(srv.Close)

			client := NewHTTPClient(cfg, maxConns, log)

			// burst sends requests concurrently and returns how many were in
			// flight at the server at the same time.
			burst := func() int {
				g := &gate{allIn: make(chan struct{}), release: make(chan struct{})}
				mu.Lock()
				cur, peak = g, 0
				mu.Unlock()

				var wg sync.WaitGroup
				for i := 0; i < requests; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						resp, err := client.Get(srv.URL)
						if err == nil {
							resp.Body.Close()
						}
					}()
				}
				select {
				case <-g.allIn:
					time.Sleep(50 * time.Millisecond) // let extra requests show up
				case <-time.After(5 * time.Second):
				}
				close(g.release)
				wg.Wait()

				mu.Lock()
				defer mu.Unlock()
				return peak
			}

			assert.Equal(t, tc.want, burst(), "requests in flight at the server, first burst")
			assert.Equal(t, tc.want, burst(), "requests in flight at the server, second burst")
			assert.Equal(t, int32(tc.want), newConns.Load(), "connections accepted by the server over two bursts")
		})
	}
}
