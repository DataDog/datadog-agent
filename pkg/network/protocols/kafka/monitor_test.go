// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func skipTestIfKernelNotSupported(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < MinimumKernelVersion {
		t.Skip(fmt.Sprintf("Kafka feature not available on pre %s kernels", MinimumKernelVersion.String()))
	}
}

// This test loads the Kafka binary, produce and fetch kafka messages and verifies that we capture them
func TestSanity(t *testing.T) {
	t.Skip("We cannot set up a Kafka cluster in the test environment because of dockerhub rate limiter")
	skipTestIfKernelNotSupported(t)

	cfg := config.New()
	cfg.BPFDebug = true
	monitor, err := NewMonitor(cfg)
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	// Assuming a kafka cluster is up and running

	// to produce/consume messages
	topic := strings.Repeat("t", 50)
	partition := 0

	myDialer := kafka.DefaultDialer
	myDialer.ClientID = "test-client-id"

	conn, err := myDialer.DialLeader(context.Background(), "tcp", "127.0.0.1:9092", topic, partition)
	require.NoError(t, err)

	err = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	require.NoError(t, err)
	_, err = conn.WriteMessages(
		kafka.Message{Value: []byte("one!")},
	)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   []string{"127.0.0.1:9092"},
		Topic:     topic,
		Partition: 0,
		MinBytes:  10e3, // 10KB
		MaxBytes:  10e6, // 10MB
	})
	err = r.SetOffset(0)
	require.NoError(t, err)

	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	for {
		m, err := r.ReadMessage(ctxTimeout)
		if err != nil {
			break
		}
		fmt.Printf("message at offset %d: %s = %s\n", m.Offset, string(m.Key), string(m.Value))
	}
	require.NoError(t, r.Close())

	kafkaStats := monitor.GetKafkaStats()
	// We expect 2 occurrences for each connection as we are working with a docker for now
	require.Equal(t, 4, len(kafkaStats))
	for _, kafkaStat := range kafkaStats {
		// When the ctxTimeout is configured with 10 seconds, we get 2 fetches from this client
		kafkaStatIsOK := kafkaStat.Data[ProduceAPIKey].Count == 1 || kafkaStat.Data[FetchAPIKey].Count == 2
		// TODO: need to add the kafka_seen_before so we won't get too much requests
		require.True(t, kafkaStatIsOK)
	}
}

// This test will help us identify if there is any verifier problems while loading the Kafka binary in the CI environment
func TestLoadKafkaBinary(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	cfg := config.New()
	monitor, err := NewMonitor(cfg)
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()
}

// This test will help us identify if there is any verifier problems while loading the Kafka binary in the CI environment
func TestLoadKafkaDebugBinary(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	cfg := config.New()
	cfg.BPFDebug = true
	monitor, err := NewMonitor(cfg)
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()
}
