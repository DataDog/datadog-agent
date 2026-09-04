// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package kernel

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"golang.org/x/sys/unix"
)

func TestParseKmsgRecord(t *testing.T) {
	record, err := parseKmsgRecord([]byte("30,42,123456,c,ignored;NVRM: Xid\n"))

	require.NoError(t, err)
	require.Equal(t, uint8(3), record.Facility)
	require.Equal(t, uint8(6), record.Priority)
	require.Equal(t, uint64(42), record.Sequence)
	require.Equal(t, uint64(123456), record.Timestamp)
	require.Equal(t, "c", record.Flags)
	require.Equal(t, "NVRM: Xid", record.Message)
}

func TestParseKmsgRecordRejectsMalformedRecords(t *testing.T) {
	for _, record := range [][]byte{
		[]byte("30,42,123456,-"),
		[]byte("30,42;message"),
		[]byte("bad,42,123456,-;message"),
		[]byte("30,bad,123456,-;message"),
		[]byte("30,42,bad,-;message"),
	} {
		_, err := parseKmsgRecord(record)
		require.Error(t, err, "record %q", record)
	}
}

func TestKmsgReaderFiltersRecordsAndCountsMalformedRecords(t *testing.T) {
	source := newFakeKmsgSource()
	tel := telemetrymock.New(t)
	reader := newTestKmsgReader(t, source, tel, 2)
	records, _ := subscribeTest(t, reader, "filtered", func(record KmsgRecord) bool {
		return record.Message == "keep"
	})

	source.enqueue(recordResult("malformed"))
	source.enqueue(recordResult("6,1,100,-;keep\n"))
	source.enqueue(recordResult("6,2,101,-;drop\n"))
	source.enqueue(recordResult("6,4,102,-;keep\n"))

	require.Equal(t, uint64(1), receiveRecord(t, records).Sequence)
	require.Equal(t, uint64(4), receiveRecord(t, records).Sequence)
	require.Eventually(t, func() bool {
		return counterValue(t, tel, "records_read") == 4 &&
			counterValue(t, tel, "records_delivered") == 2 &&
			counterValue(t, tel, "errors") == 1
	}, time.Second, time.Millisecond)
}

func TestKmsgReaderRecoversFromOverrun(t *testing.T) {
	source := newFakeKmsgSource()
	tel := telemetrymock.New(t)
	reader := newTestKmsgReader(t, source, tel, 1)
	records, _ := subscribeTest(t, reader, "all", nil)

	source.enqueue(readResult{err: unix.EPIPE})
	source.enqueue(recordResult("6,9,100,-;keep\n"))

	require.Equal(t, uint64(9), receiveRecord(t, records).Sequence)
	require.Eventually(t, func() bool {
		return counterValue(t, tel, "ring_buffer_losses") == 1
	}, time.Second, time.Millisecond)
}

func TestKmsgReaderCountsRecordsDroppedFromFullChannel(t *testing.T) {
	source := newFakeKmsgSource()
	tel := telemetrymock.New(t)
	reader := newTestKmsgReader(t, source, tel, 1)
	records, _ := subscribeTest(t, reader, "slow", nil)

	source.enqueue(recordResult("6,1,100,-;first\n"))
	source.enqueue(recordResult("6,2,101,-;second\n"))

	require.Eventually(t, func() bool {
		return counterValueOrZero(tel, "losses") == 1
	}, time.Second, time.Millisecond)
	require.Equal(t, float64(1), subscriberCounterValue(t, tel, "losses", "slow"))
	require.Equal(t, "first", receiveRecord(t, records).Message)
}

func TestKmsgReaderBroadcastsToMatchingSubscribers(t *testing.T) {
	source := newFakeKmsgSource()
	tel := telemetrymock.New(t)
	reader := newTestKmsgReader(t, source, tel, 1)
	allRecords, _ := subscribeTest(t, reader, "all", nil)
	keepRecords, _ := subscribeTest(t, reader, "keep", func(record KmsgRecord) bool {
		return record.Message == "keep"
	})

	source.enqueue(recordResult("6,1,100,-;keep\n"))

	require.Equal(t, uint64(1), receiveRecord(t, allRecords).Sequence)
	require.Equal(t, uint64(1), receiveRecord(t, keepRecords).Sequence)
	require.Eventually(t, func() bool {
		return subscriberCounterValue(t, tel, "records_delivered", "all") == 1 &&
			subscriberCounterValue(t, tel, "records_delivered", "keep") == 1
	}, time.Second, time.Millisecond)
}

func TestKmsgReaderUnsubscribes(t *testing.T) {
	source := newFakeKmsgSource()
	reader := newTestKmsgReader(t, source, telemetrymock.New(t), 1)
	removedRecords, unsubscribe := subscribeTest(t, reader, "removed", nil)
	remainingRecords, _ := subscribeTest(t, reader, "remaining", nil)

	unsubscribe()
	unsubscribe()

	_, ok := <-removedRecords
	require.False(t, ok)

	source.enqueue(recordResult("6,1,100,-;keep\n"))
	require.Equal(t, uint64(1), receiveRecord(t, remainingRecords).Sequence)
}

func TestKmsgReaderUnsubscribeAfterStop(t *testing.T) {
	source := newFakeKmsgSource()
	reader := newTestKmsgReader(t, source, telemetrymock.New(t), 1)
	records, unsubscribe := subscribeTest(t, reader, "subscriber", nil)

	reader.Stop()
	unsubscribe()
	unsubscribe()

	_, ok := <-records
	require.False(t, ok)
}

func TestKmsgReaderRejectsInvalidSubscriptions(t *testing.T) {
	source := newFakeKmsgSource()
	reader := newTestKmsgReader(t, source, telemetrymock.New(t), 1)

	_, _, err := reader.Subscribe("", nil)
	require.ErrorContains(t, err, "subscriber name is empty")

	_, _ = subscribeTest(t, reader, "duplicate", nil)
	_, _, err = reader.Subscribe("duplicate", nil)
	require.ErrorContains(t, err, `subscriber "duplicate" already exists`)

	reader.Stop()
	_, _, err = reader.Subscribe("stopped", nil)
	require.ErrorContains(t, err, "reader is stopped")
}

func TestKmsgReaderConcurrentSubscribeUnsubscribeAndStop(t *testing.T) {
	const (
		recordCount     = 250_000
		subscriberCount = 16
		subscriptions   = 100
	)

	source := newFakeKmsgSource()
	tel := telemetrymock.New(t)
	reader := newTestKmsgReader(t, source, tel, 4)
	persistentRecords, _ := subscribeTest(t, reader, "persistent", nil)

	persistentClosed := make(chan struct{})
	go func() {
		for {
			if _, ok := <-persistentRecords; !ok {
				break
			}
		}
		close(persistentClosed)
	}()

	var producers sync.WaitGroup
	producers.Add(1)
	go func() {
		defer producers.Done()
		for i := 0; i < recordCount; i++ {
			source.enqueue(recordResult(fmt.Sprintf("6,%d,100,-;record\n", i)))
		}
	}()

	subscriberErrors := make(chan error, subscriberCount)
	var subscribers sync.WaitGroup
	for i := 0; i < subscriberCount; i++ {
		subscribers.Add(1)
		go func(id int) {
			defer subscribers.Done()
			name := fmt.Sprintf("concurrent-%d", id)
			for range subscriptions {
				_, unsubscribe, err := reader.Subscribe(name, nil)
				if err != nil {
					subscriberErrors <- err
					return
				}
				unsubscribe()
			}
		}(i)
	}

	producers.Wait()
	subscribers.Wait()
	close(subscriberErrors)
	for err := range subscriberErrors {
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool {
		return counterValue(t, tel, "records_read") == recordCount
	}, time.Second, time.Millisecond)

	start := make(chan struct{})
	var shutdown sync.WaitGroup
	for i := 0; i < subscriberCount; i++ {
		shutdown.Add(1)
		go func(id int) {
			defer shutdown.Done()
			<-start
			_, unsubscribe, err := reader.Subscribe(fmt.Sprintf("shutdown-%d", id), nil)
			if err == nil {
				unsubscribe()
			}
		}(i)
	}
	shutdown.Add(1)
	go func() {
		defer shutdown.Done()
		<-start
		reader.Stop()
	}()

	close(start)
	shutdown.Wait()

	select {
	case <-persistentClosed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for persistent subscriber to close")
	}
}

func TestKmsgReaderReportsTerminalReadError(t *testing.T) {
	source := newFakeKmsgSource()
	tel := telemetrymock.New(t)
	reader := newTestKmsgReader(t, source, tel, 1)

	source.enqueue(readResult{err: errors.New("read failed")})

	require.ErrorContains(t, receiveError(t, reader.Errors()), "read kmsg record: read failed")
	require.Eventually(t, func() bool {
		return counterValue(t, tel, "errors") == 1
	}, time.Second, time.Millisecond)
}

func TestKmsgReaderReportsEmptyRead(t *testing.T) {
	source := newFakeKmsgSource()
	tel := telemetrymock.New(t)
	reader := newTestKmsgReader(t, source, tel, 1)

	source.enqueue(readResult{})

	require.ErrorContains(t, receiveError(t, reader.Errors()), "read kmsg record returned no data")
	require.Eventually(t, func() bool {
		return counterValue(t, tel, "errors") == 1
	}, time.Second, time.Millisecond)
}

func TestKmsgReaderStopCancelsIdleRead(t *testing.T) {
	source := newFakeKmsgSource()
	reader := newTestKmsgReader(t, source, telemetrymock.New(t), 1)

	stopped := make(chan struct{})
	go func() {
		reader.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("timed out stopping kmsg reader")
	}
}

func TestNewKmsgReaderRejectsInvalidConfiguration(t *testing.T) {
	tel := telemetrymock.New(t)
	kmsgTelemetry := &kmsgTelemetry{}
	kmsgTelemetry.init(tel)
	_, err := newKmsgReader(nil, kmsgTelemetry, 1)
	require.ErrorContains(t, err, "kmsg source is nil")

	source := newFakeKmsgSource()
	_, err = newKmsgReader(source, kmsgTelemetry, 0)
	require.ErrorContains(t, err, "channel size must be positive")

	source = newFakeKmsgSource()
	source.seekErr = errors.New("seek failed")
	_, err = newKmsgReader(source, kmsgTelemetry, 1)
	require.ErrorContains(t, err, "seek kmsg to end: seek failed")
}

func newTestKmsgReader(t *testing.T, source *fakeKmsgSource, component telemetry.Component, channelSize int) *KmsgReader {
	t.Helper()

	kmsgTelemetry := &kmsgTelemetry{}
	kmsgTelemetry.init(component)

	reader, err := newKmsgReader(source, kmsgTelemetry, channelSize)
	require.NoError(t, err)
	t.Cleanup(reader.Stop)
	return reader
}

func subscribeTest(t *testing.T, reader *KmsgReader, name string, filter KmsgFilter) (<-chan KmsgRecord, func()) {
	t.Helper()

	records, unsubscribe, err := reader.Subscribe(name, filter)
	require.NoError(t, err)
	return records, unsubscribe
}

func receiveRecord(t *testing.T, records <-chan KmsgRecord) KmsgRecord {
	t.Helper()

	select {
	case record := <-records:
		return record
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for kmsg record")
		return KmsgRecord{}
	}
}

func receiveError(t *testing.T, errors <-chan error) error {
	t.Helper()

	select {
	case err := <-errors:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for kmsg reader error")
		return nil
	}
}

func counterValue(t *testing.T, component telemetry.Mock, name string) float64 {
	t.Helper()

	metrics, err := component.GetCountMetric("kernel__kmsg", name)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	return metrics[0].Value()
}

func counterValueOrZero(component telemetry.Mock, name string) float64 {
	metrics, err := component.GetCountMetric("kernel__kmsg", name)
	if err != nil || len(metrics) != 1 {
		return 0
	}
	return metrics[0].Value()
}

func subscriberCounterValue(t *testing.T, component telemetry.Mock, name, subscriber string) float64 {
	t.Helper()

	metrics, err := component.GetCountMetric("kernel__kmsg", name)
	require.NoError(t, err)
	for _, metric := range metrics {
		if metric.Tags()["subscriber"] == subscriber {
			return metric.Value()
		}
	}
	t.Fatalf("metric %q has no value for subscriber %q", name, subscriber)
	return 0
}

type readResult struct {
	record string
	err    error
}

func recordResult(record string) readResult {
	return readResult{record: record}
}

type fakeKmsgSource struct {
	events chan readResult

	closeOnce sync.Once
	seekErr   error
}

func newFakeKmsgSource() *fakeKmsgSource {
	return &fakeKmsgSource{events: make(chan readResult, 16)}
}

func (s *fakeKmsgSource) enqueue(result readResult) {
	s.events <- result
}

func (s *fakeKmsgSource) Read(buffer []byte) (int, error) {
	result, ok := <-s.events
	if !ok {
		return 0, os.ErrClosed
	}
	if result.err != nil {
		return 0, result.err
	}
	return copy(buffer, result.record), nil
}

func (s *fakeKmsgSource) Seek(_ int64, _ int) (int64, error) {
	return 0, s.seekErr
}

func (s *fakeKmsgSource) Close() error {
	s.closeOnce.Do(func() {
		close(s.events)
	})
	return nil
}

var _ kmsgSource = (*fakeKmsgSource)(nil)
