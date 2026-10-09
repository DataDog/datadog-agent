// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || (darwin && cgo)

package checksfit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit/fitcore"
)

// newSocketPath returns a setup-socket path inside a private directory: FIT
// requires the socket's parent to deny group and other access, and the path
// must stay short enough for a unix socket (about 100 bytes on macOS).
func newSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "aadfit")
	if err != nil {
		t.Fatalf("create socket dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("restrict socket dir: %v", err)
	}
	return filepath.Join(dir, "aad.sock")
}

func TestProducerAndConsumerExchangeMetrics(t *testing.T) {
	path := newSocketPath(t)
	endpoint, err := fitcore.ParseEndpoint("unix:" + path)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	var (
		wg         sync.WaitGroup
		sessionID  uint64
		received   []Metric
		consumerEr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer, err := Open(fitcore.NewConsumerConfig(endpoint))
		if err != nil {
			consumerEr = err
			return
		}
		defer consumer.Close()
		sessionID = consumer.SessionID()
		for index := 0; index < 3; index++ {
			metric, err := consumer.Receive()
			if err != nil {
				consumerEr = err
				return
			}
			received = append(received, metric)
		}
	}()

	producer, err := connectWithRetry(endpoint)
	if err != nil {
		t.Fatalf("connect producer: %v", err)
	}
	defer producer.Close()

	want := []Metric{
		{MetricType: MetricTypeCounter, Name: "log.pattern.abc.count", Value: 5, Timestamp: 100, Tags: []string{"observer_source:logs"}, Hostname: "web-1"},
		{MetricType: MetricTypeCounter, Name: "log.log_pattern_extractor.def.count", Value: 3, Timestamp: 100, Hostname: "web-1"},
		{MetricType: MetricTypeGauge, Name: "log.field.duration_ms", Value: 12.5, Timestamp: 101, Hostname: "web-2"},
	}
	outcome, err := producer.SendMetrics(want)
	if err != nil {
		t.Fatalf("SendMetrics: %v", err)
	}
	if outcome.Accepted != len(want) || outcome.QueueRejection != fitcore.RejectionNone || outcome.EncodingError != nil {
		t.Fatalf("outcome = %+v, want all %d records accepted without rejection or encoding error", outcome, len(want))
	}

	wg.Wait()
	if consumerEr != nil {
		t.Fatalf("consumer: %v", consumerEr)
	}
	if producer.SessionID() != sessionID {
		t.Fatalf("producer session %d, consumer session %d; want the same session", producer.SessionID(), sessionID)
	}
	if len(received) != len(want) {
		t.Fatalf("received %d metrics, want %d", len(received), len(want))
	}
	for index := range want {
		if !metricsEqual(received[index], want[index]) {
			t.Fatalf("metric %d = %+v, want %+v", index, received[index], want[index])
		}
	}
}

func TestSendMetricsReportsEncodingErrors(t *testing.T) {
	restore := lowerPayloadLimit(t, 64)
	defer restore()

	path := newSocketPath(t)
	endpoint, err := fitcore.ParseEndpoint("unix:" + path)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	ready := make(chan struct{})
	go func() {
		consumer, err := Open(fitcore.NewConsumerConfig(endpoint))
		if err != nil {
			close(ready)
			return
		}
		defer consumer.Close()
		close(ready)
		// Drain the one encodable record so the producer never blocks on a full ring.
		_, _ = consumer.ReceiveContext(timeoutContext(t, 5*time.Second))
		_, _ = consumer.ReceiveContext(timeoutContext(t, 5*time.Second))
	}()

	producer, err := connectWithRetry(endpoint)
	if err != nil {
		t.Fatalf("connect producer: %v", err)
	}
	defer producer.Close()

	outcome, err := producer.SendMetrics([]Metric{
		{MetricType: MetricTypeCounter, Name: "ok.count", Value: 1, Timestamp: 1},
		{Name: strings.Repeat("a", 128)},
	})
	if err != nil {
		t.Fatalf("SendMetrics: %v", err)
	}
	if outcome.Accepted != 1 {
		t.Fatalf("accepted = %d, want 1 (the encodable prefix)", outcome.Accepted)
	}
	if !errors.Is(outcome.EncodingError, errPayloadTooLarge) {
		t.Fatalf("encoding error = %v, want %v", outcome.EncodingError, errPayloadTooLarge)
	}
	<-ready
}

func TestConnectFailsWhenNoConsumerIsListening(t *testing.T) {
	path := newSocketPath(t)
	endpoint, err := fitcore.ParseEndpoint("unix:" + path)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	config := fitcore.NewProducerConfig(endpoint)
	config.SetupTimeout = 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := ConnectContext(ctx, config); err == nil {
		t.Fatalf("ConnectContext succeeded without a consumer")
	}
}

// connectWithRetry waits for the consumer's setup socket to start listening.
func connectWithRetry(endpoint fitcore.SetupEndpoint) (*Producer, error) {
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		producer, err := Connect(fitcore.NewProducerConfig(endpoint))
		if err == nil {
			return producer, nil
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	return nil, lastErr
}

func timeoutContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}
