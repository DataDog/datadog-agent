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
	"io"
	nethttp "net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kversion"
)

var defaultTopicName = "franz-kafka"

type BinaryType int

const (
	PREBUILT = 0
	RUNTIME  = 1
	CORE     = 2
)

// This test loads the Kafka binary, produce and fetch kafka messages and verifies that we capture them
func TestSanity(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	kafka.RunServer(t, "127.0.0.1", "9092")
	monitor := newHTTPWithKafkaMonitor(t)

	seeds := []string{"localhost:9092"}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.DefaultProduceTopic(defaultTopicName),
		kgo.ConsumeTopics(defaultTopicName),
		kgo.MaxVersions(kversion.V2_5_0()),
	)
	require.NoError(t, err)
	defer client.Close()
	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
	err = client.Ping(ctxTimeout)
	cancel()
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

	fetches := client.PollFetches(context.Background())
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
		require.Equal(t, defaultTopicName, kafkaKey.TopicName)
		switch kafkaKey.RequestAPIKey {
		case kafka.ProduceAPIKey:
			require.Equal(t, uint16(8), kafkaKey.RequestVersion)
			numberOfProduceRequests += kafkaStat.Count
			break
		case kafka.FetchAPIKey:
			require.Equal(t, uint16(11), kafkaKey.RequestVersion)
			numberOfFetchRequests += kafkaStat.Count
		default:
			require.FailNow(t, "Expecting only produce or fetch kafka requests")
		}
	}
	kafkaStatIsOK := numberOfProduceRequests == 2 || numberOfFetchRequests == 2
	require.True(t, kafkaStatIsOK, "Number of produce requests: %d, number of fetch requests: %d", numberOfProduceRequests, numberOfFetchRequests)
}

// This test will help us identify if there is any verifier problems while loading the Kafka binary in the CI environment
func TestLoadKafkaBinary(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	t.Run("prebuilt release binary", func(t *testing.T) {
		loadKafkaBinary(t, false, PREBUILT)
	})
	t.Run("prebuilt debug binary", func(t *testing.T) {
		loadKafkaBinary(t, true, PREBUILT)
	})

	t.Run("runtime release binary", func(t *testing.T) {
		loadKafkaBinary(t, false, RUNTIME)
	})
	t.Run("runtime debug binary", func(t *testing.T) {
		loadKafkaBinary(t, true, RUNTIME)
	})

	t.Run("CO-RE release binary", func(t *testing.T) {
		loadKafkaBinary(t, false, CORE)
	})
	t.Run("CO-RE debug binary", func(t *testing.T) {
		loadKafkaBinary(t, true, CORE)
	})
}

func loadKafkaBinary(t *testing.T, debug bool, binaryType BinaryType) {
	cfg := config.New()
	// We don't have a way of enabling kafka without http at the moment
	cfg.EnableHTTPMonitoring = true
	cfg.EnableKafkaMonitoring = true
	cfg.BPFDebug = debug

	cfg.AllowPrecompiledFallback = false
	cfg.AllowRuntimeCompiledFallback = false
	cfg.EnableCORE = false
	switch binaryType {
	case PREBUILT:
		cfg.AllowPrecompiledFallback = true
	case RUNTIME:
		cfg.AllowRuntimeCompiledFallback = true
	case CORE:
		cfg.EnableCORE = true
	}

	newHTTPWithKafkaMonitor(t)
}

func TestProduceClientIdEmptyString(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	kafka.RunServer(t, "127.0.0.1", "9092")
	monitor := newHTTPWithKafkaMonitor(t)

	seeds := []string{"localhost:9092"}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.DefaultProduceTopic(defaultTopicName),
		kgo.MaxVersions(kversion.V1_0_0()),
		kgo.ClientID(""),
	)
	require.NoError(t, err)
	defer client.Close()
	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
	err = client.Ping(ctxTimeout)
	cancel()
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
		require.Equal(t, 1, kafkaStat.Count)
	}
}

func TestManyProduceRequests(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	kafka.RunServer(t, "127.0.0.1", "9092")
	monitor := newHTTPWithKafkaMonitor(t)

	seeds := []string{"localhost:9092"}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.DefaultProduceTopic(defaultTopicName),
		kgo.MaxVersions(kversion.V2_5_0()),
	)
	require.NoError(t, err)
	defer client.Close()
	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
	err = client.Ping(ctxTimeout)
	cancel()
	require.NoError(t, err)

	// Create the topic
	adminClient := kadm.NewClient(client)
	ctxTimeout, cancel = context.WithTimeout(context.Background(), time.Second*5)
	_, err = adminClient.CreateTopics(ctxTimeout, 1, 1, nil, defaultTopicName)
	cancel()
	require.NoError(t, err)

	numberOfIterations := 1000
	for i := 1; i <= numberOfIterations; i++ {
		record := &kgo.Record{Topic: defaultTopicName, Value: []byte("Hello Kafka!")}
		ctxTimeout, cancel = context.WithTimeout(context.Background(), time.Second*5)
		err = client.ProduceSync(ctxTimeout, record).FirstErr()
		cancel()
		require.NoError(t, err, "record had a produce error while synchronously producing")
	}

	// Wait for the kafka monitor to process the Kafka traffic
	time.Sleep(time.Second * 2)

	kafkaStats := monitor.GetKafkaStats()
	// We expect 2 occurrences for each connection as we are working with a docker for now
	require.Equal(t, 2, len(kafkaStats))
	for kafkaKey, kafkaStat := range kafkaStats {
		if kafkaKey.RequestAPIKey != kafka.ProduceAPIKey {
			require.FailNow(t, "Expecting only produce requests")
		}
		require.Equal(t, numberOfIterations, kafkaStat.Count)
	}
}

func TestHTTPAndKafka(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	kafka.RunServer(t, "127.0.0.1", "9092")
	monitor := newHTTPWithKafkaMonitor(t)

	seeds := []string{"localhost:9092"}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.DefaultProduceTopic(defaultTopicName),
		kgo.MaxVersions(kversion.V2_5_0()),
	)
	require.NoError(t, err)
	defer client.Close()
	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
	err = client.Ping(ctxTimeout)
	cancel()
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

	serverAddr := "localhost:8081"
	srvDoneFn := testutil.HTTPServer(t, "localhost:8081", testutil.Options{})
	httpClient := nethttp.Client{}

	req, err := nethttp.NewRequest(httpMethods[0], fmt.Sprintf("http://%s/%d/request", serverAddr, nethttp.StatusOK), nil)
	require.NoError(t, err)

	expectedOccurrences := 10
	for i := 0; i < expectedOccurrences; i++ {
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		// Have to read the response body to ensure the client will be able to properly close the connection.
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	srvDoneFn()

	// Wait for the kafka monitor to process the Kafka traffic
	time.Sleep(time.Second * 2)

	occurrences := 0
	require.Eventually(t, func() bool {
		httpStats := monitor.GetHTTPStats()
		occurrences += countRequestOccurrences(httpStats, req)
		return occurrences == expectedOccurrences
	}, time.Second*3, time.Millisecond*100, "Expected to find a request %d times, instead captured %d", expectedOccurrences, occurrences)

	kafkaStats := monitor.GetKafkaStats()
	// We expect 2 occurrences for each connection as we are working with a docker for now
	require.Equal(t, 2, len(kafkaStats))
	for kafkaKey, kafkaStat := range kafkaStats {
		if kafkaKey.RequestAPIKey != kafka.ProduceAPIKey {
			require.FailNow(t, "Expecting only produce requests")
		}
		require.Equal(t, 1, kafkaStat.Count)
	}
}

func TestEnableHTTPOnly(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	kafka.RunServer(t, "127.0.0.1", "9092")
	monitor := newHTTPMonitor(t)

	seeds := []string{"localhost:9092"}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.DefaultProduceTopic(defaultTopicName),
		kgo.MaxVersions(kversion.V1_0_0()),
	)
	require.NoError(t, err)
	defer client.Close()
	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
	err = client.Ping(ctxTimeout)
	cancel()
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
	require.Equal(t, 0, len(kafkaStats))
}

func skipTestIfKernelNotSupported(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < MinimumKernelVersion {
		t.Skip(fmt.Sprintf("Kafka feature not available on pre %s kernels", MinimumKernelVersion.String()))
	}
}

func newHTTPWithKafkaMonitor(t *testing.T) *Monitor {
	cfg := config.New()
	// We don't have a way of enabling kafka without http at the moment
	cfg.EnableHTTPMonitoring = true
	cfg.EnableKafkaMonitoring = true
	monitor, err := NewMonitor(cfg, nil, nil, nil)
	skipIfNotSupported(t, err)
	require.NoError(t, err)
	t.Cleanup(monitor.Stop)

	err = monitor.Start()
	skipIfNotSupported(t, err)
	require.NoError(t, err)
	return monitor
}
