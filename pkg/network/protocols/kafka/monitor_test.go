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
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kversion"
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
	skipTestIfKernelNotSupported(t)

	RunKafkaServer(t, "127.0.0.1", "9092")

	cfg := config.New()
	cfg.BPFDebug = true
	monitor, err := NewMonitor(cfg, nil)
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	topicName := "franz-kafka"
	seeds := []string{"localhost:9092"}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.DefaultProduceTopic(topicName),
		//kgo.ConsumerGroup("my-group-identifier"),
		kgo.ConsumeTopics(topicName),
		kgo.MaxVersions(kversion.V1_0_0()),
	)
	require.NoError(t, err)
	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
	err = client.Ping(ctxTimeout)
	cancel()
	defer client.Close()
	require.NoError(t, err)

	// Create the topic
	adminClient := kadm.NewClient(client)
	ctxTimeout, cancel = context.WithTimeout(context.Background(), time.Second*5)
	_, err = adminClient.CreateTopics(ctxTimeout, 1, 1, nil, topicName)
	cancel()
	require.NoError(t, err)

	record := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka!")}
	ctxTimeout, cancel = context.WithTimeout(context.Background(), time.Second*5)
	err = client.ProduceSync(ctxTimeout, record).FirstErr()
	cancel()
	require.NoError(t, err, "record had a produce error while synchronously producing")

	//ctxTimeout, cancel = context.WithTimeout(context.Background(), time.Second*5)
	fetches := client.PollFetches(nil)
	//cancel()
	errs := fetches.Errors()
	for _, err := range errs {
		t.Errorf("PollFetches error: %+v", err)
		t.FailNow()
	}

	// Wait for the kafka monitor to process the Kafka traffic
	time.Sleep(time.Second * 2)

	kafkaStats := monitor.GetKafkaStats()
	// We expect 2 occurrences for each connection as we are working with a docker for now
	require.Equal(t, 4, len(kafkaStats))
	for _, kafkaStat := range kafkaStats {
		// When the ctxTimeout is configured with 10 seconds, we get 2 fetches from this client
		produceRequestsCount := kafkaStat.Data[ProduceAPIKey].Count
		fetchRequestsCount := kafkaStat.Data[FetchAPIKey].Count
		kafkaStatIsOK := produceRequestsCount == 1 || fetchRequestsCount == 1
		require.True(t, kafkaStatIsOK, "Number of produce requests: %d, number of fetch requests: %d", produceRequestsCount, fetchRequestsCount)
	}
}

// This test will help us identify if there is any verifier problems while loading the Kafka binary in the CI environment
func TestLoadKafkaBinary(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	cfg := config.New()
	monitor, err := NewMonitor(cfg, nil)
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
	monitor, err := NewMonitor(cfg, nil)
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()
}
