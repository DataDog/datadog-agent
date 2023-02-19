// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kversion"
)

var defaultTopicName = "franz-kafka"

func skipTestIfKernelNotSupported(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < kafka.MinimumKernelVersion {
		t.Skip(fmt.Sprintf("Kafka feature not available on pre %s kernels", kafka.MinimumKernelVersion.String()))
	}
}

/*
TODO tests:
* Enable USM, make sure kafka monitoring is not enabled
*/

// This test loads the Kafka binary, produce and fetch kafka messages and verifies that we capture them
func TestSanity(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	kafka.RunKafkaServer(t, "127.0.0.1", "9092")

	cfg := config.New()
	// We don't have a way of enabling kafka without http at the moment
	cfg.EnableHTTPMonitoring = true
	cfg.BPFDebug = true
	monitor, err := NewMonitor(cfg, nil, nil, nil)
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	seeds := []string{"localhost:9092"}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.DefaultProduceTopic(defaultTopicName),
		//kgo.ConsumerGroup("my-group-identifier"),
		kgo.ConsumeTopics(defaultTopicName),
		kgo.MaxVersions(kversion.V2_5_0()),
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
	_, err = adminClient.CreateTopics(ctxTimeout, 1, 1, nil, defaultTopicName)
	cancel()
	require.NoError(t, err)

	record := &kgo.Record{Topic: defaultTopicName, Value: []byte("Hello Kafka!")}
	ctxTimeout, cancel = context.WithTimeout(context.Background(), time.Second*5)
	err = client.ProduceSync(ctxTimeout, record).FirstErr()
	cancel()
	require.NoError(t, err, "record had a produce error while synchronously producing")

	fetches := client.PollFetches(nil)
	errs := fetches.Errors()
	for _, err := range errs {
		t.Errorf("PollFetches error: %+v", err)
		t.FailNow()
	}

	// Wait for the kafka monitor to process the Kafka traffic
	time.Sleep(time.Second * 2)

	kafkaStats := monitor.GetKafkaStats()
	// We expect 2 occurrences for each connection as we are working with a docker
	require.Equal(t, 4, len(kafkaStats))
	numberOfProduceRequests := 0
	numberOfFetchRequests := 0
	for kafkaKey, kafkaStat := range kafkaStats {
		// When the ctxTimeout is configured with 10 seconds, we get 2 fetches from this client
		if kafkaKey.RequestAPIKey == kafka.ProduceAPIKey {
			numberOfProduceRequests += kafkaStat.Count
		} else if kafkaKey.RequestAPIKey == kafka.FetchAPIKey {
			numberOfFetchRequests += kafkaStat.Count
		} else {
			require.FailNow(t, "Expecting only produce or fetch kafka requests")
		}
	}
	kafkaStatIsOK := numberOfProduceRequests == 2 || numberOfFetchRequests == 2
	require.True(t, kafkaStatIsOK, "Number of produce requests: %d, number of fetch requests: %d", numberOfProduceRequests, numberOfFetchRequests)
}

// This test will help us identify if there is any verifier problems while loading the Kafka binary in the CI environment
func TestLoadKafkaBinary(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	cfg := config.New()
	// We don't have a way of enabling kafka without http at the moment
	cfg.EnableHTTPMonitoring = true
	monitor, err := NewMonitor(cfg, nil, nil, nil)
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
	// We don't have a way of enabling kafka without http at the moment
	cfg.EnableHTTPMonitoring = true
	monitor, err := NewMonitor(cfg, nil, nil, nil)
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()
}

func TestProduceClientIdEmptyString(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	kafka.RunKafkaServer(t, "127.0.0.1", "9092")

	cfg := config.New()
	cfg.BPFDebug = true
	// We don't have a way of enabling kafka without http at the moment
	cfg.EnableHTTPMonitoring = true
	monitor, err := NewMonitor(cfg, nil, nil, nil)
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	seeds := []string{"localhost:9092"}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.DefaultProduceTopic(defaultTopicName),
		kgo.MaxVersions(kversion.V1_0_0()),
		//V3_0_0
		kgo.ClientID(""),
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
	_, err = adminClient.CreateTopics(ctxTimeout, 1, 1, nil, defaultTopicName)
	cancel()
	require.NoError(t, err)

	record := &kgo.Record{Topic: defaultTopicName, Value: []byte("Hello Kafka!")}
	ctxTimeout, cancel = context.WithTimeout(context.Background(), time.Second*5)
	err = client.ProduceSync(ctxTimeout, record).FirstErr()
	cancel()
	require.NoError(t, err, "record had a produce error while synchronously producing")

	// Wait for the kafka monitor to process the Kafka traffic
	time.Sleep(time.Second * 2)

	kafkaStats := monitor.GetKafkaStats()
	// We expect 2 occurrences for each connection as we are working with a docker for now
	require.Equal(t, 2, len(kafkaStats))
	for kafkaKey, kafkaStat := range kafkaStats {
		if kafkaKey.RequestAPIKey != kafka.ProduceAPIKey {
			require.FailNow(t, "Expecting only produce requests")
		}
		produceRequestsCount := kafkaStat.Count
		kafkaStatIsOK := produceRequestsCount == 1
		require.True(t, kafkaStatIsOK, "Number of produce kafka requests: %d", produceRequestsCount)
	}
}
