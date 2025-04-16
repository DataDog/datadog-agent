// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package tests

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kversion"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
)

func getProduceVersionTest(produceVersion int16, targetAddress, serverAddress, topicName string, dialer *net.Dialer) protocolClassificationAttributes {
	produceExpectedStack := &protocols.Stack{Application: protocols.Kafka}

	if produceVersion < kafka.MinSupportedProduceRequestApiVersion || produceVersion > kafka.MaxSupportedProduceRequestApiVersion {
		produceExpectedStack.Application = protocols.Unknown
	}

	version := kversion.V3_8_0()
	version.SetMaxKeyVersion(produceAPIKey, produceVersion)

	return protocolClassificationAttributes{
		name: fmt.Sprintf("produce v%d", produceVersion),
		context: testContext{
			serverPort:    kafkaPort,
			targetAddress: targetAddress,
			serverAddress: serverAddress,
			extras: map[string]interface{}{
				"topic_name": topicName,
			},
		},
		preTracerSetup: func(t *testing.T, ctx testContext) {
			produceClient, err := kafka.NewClient(kafka.Options{
				ServerAddress: ctx.targetAddress,
				DialFn:        dialer.DialContext,
				CustomOptions: []kgo.Opt{kgo.MaxVersions(version)},
			})
			require.NoError(t, err)
			ctx.extras["produce_client"] = produceClient
			require.NoError(t, produceClient.CreateTopic(ctx.extras["topic_name"].(string)))
		},
		postTracerSetup: func(t *testing.T, ctx testContext) {
			produceClient := ctx.extras["produce_client"].(*kafka.Client)
			record := &kgo.Record{Topic: ctx.extras["topic_name"].(string), Value: []byte("Hello Kafka!")}
			ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			require.NoError(t, produceClient.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")
		},
		validation: validateProtocolConnection(produceExpectedStack),
	}
}

func getFetchVersionTest(fetchVersion int16, targetAddress, serverAddress, topicName string, dialer *net.Dialer) protocolClassificationAttributes {
	fetchExpectedStack := &protocols.Stack{Application: protocols.Kafka}

	if fetchVersion < kafka.MinSupportedFetchRequestApiVersion || fetchVersion > kafka.MaxSupportedFetchRequestApiVersion {
		fetchExpectedStack.Application = protocols.Unknown
	}

	version := kversion.V3_8_0()
	version.SetMaxKeyVersion(fetchAPIKey, fetchVersion)

	return protocolClassificationAttributes{
		name: fmt.Sprintf("fetch v%d", fetchVersion),
		context: testContext{
			serverPort:    kafkaPort,
			targetAddress: targetAddress,
			serverAddress: serverAddress,
			extras: map[string]interface{}{
				"topic_name": topicName,
			},
		},
		preTracerSetup: func(t *testing.T, ctx testContext) {
			produceClient, err := kafka.NewClient(kafka.Options{
				ServerAddress: ctx.targetAddress,
				DialFn:        dialer.DialContext,
				CustomOptions: []kgo.Opt{kgo.MaxVersions(version), kgo.ConsumeTopics(ctx.extras["topic_name"].(string))},
			})
			require.NoError(t, err)
			defer produceClient.Client.Close()
			require.NoError(t, produceClient.CreateTopic(ctx.extras["topic_name"].(string)))

			record := &kgo.Record{Topic: ctx.extras["topic_name"].(string), Value: []byte("Hello Kafka!")}
			ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			require.NoError(t, produceClient.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")

			fetchClient, err := kafka.NewClient(kafka.Options{
				ServerAddress: ctx.targetAddress,
				DialFn:        dialer.DialContext,
				CustomOptions: []kgo.Opt{kgo.MaxVersions(version), kgo.ConsumeTopics(ctx.extras["topic_name"].(string))},
			})
			require.NoError(t, err)
			ctx.extras["fetch_client"] = fetchClient
		},
		postTracerSetup: func(t *testing.T, ctx testContext) {
			fetchClient := ctx.extras["fetch_client"].(*kafka.Client)
			fetches := fetchClient.Client.PollFetches(context.Background())
			require.Empty(t, fetches.Errors())
			records := fetches.Records()
			require.Len(t, records, 1)
			require.Equal(t, ctx.extras["topic_name"].(string), records[0].Topic)
		},
		validation: validateProtocolConnection(fetchExpectedStack),
	}
}

func testKafkaProtocolClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	const topicName = "franz-kafka"
	testIndex := 0
	// Kafka does not allow us to delete topic, but to mark them for deletion, so we have to generate a unique topic
	// per a test.
	getTopicName := func() string {
		testIndex++
		return fmt.Sprintf("%s-%d", topicName, testIndex)
	}

	skipFunc := composeSkips(skipIfUsingNAT)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    kafkaPort,
		targetAddress: targetHost,
	})

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	kafkaTeardown := func(_ *testing.T, ctx testContext) {
		for key, val := range ctx.extras {
			if strings.HasSuffix(key, "client") {
				client := val.(*kafka.Client)
				client.Client.Close()
			}
		}
	}

	serverAddress := net.JoinHostPort(serverHost, kafkaPort)
	targetAddress := net.JoinHostPort(targetHost, kafkaPort)
	require.NoError(t, kafka.RunServer(t, serverHost, kafkaPort))

	tests := []protocolClassificationAttributes{
		{
			name: "connect",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        defaultDialer.DialContext,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(kversion.V0_10_1())},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			validation: validateProtocolConnection(&protocols.Stack{}),
			teardown:   kafkaTeardown,
		},
		{
			name: "create topic",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        defaultDialer.DialContext,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(kversion.V0_10_1())},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*kafka.Client)
				require.NoError(t, client.CreateTopic(ctx.extras["topic_name"].(string)))
			},
			validation: validateProtocolConnection(&protocols.Stack{}),
			teardown:   kafkaTeardown,
		},
		{
			name: "produce - empty string client id",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        defaultDialer.DialContext,
					CustomOptions: []kgo.Opt{kgo.ClientID(""), kgo.MaxVersions(kversion.V0_10_1())},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(ctx.extras["topic_name"].(string)))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*kafka.Client)
				record := &kgo.Record{Topic: ctx.extras["topic_name"].(string), Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.Kafka}),
			teardown:   kafkaTeardown,
		},
		{
			name: "produce - multiple topics",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name1": getTopicName(),
					"topic_name2": getTopicName(),
				},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        defaultDialer.DialContext,
					CustomOptions: []kgo.Opt{kgo.ClientID(""), kgo.MaxVersions(kversion.V0_10_1())},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(ctx.extras["topic_name1"].(string)))
				require.NoError(t, client.CreateTopic(ctx.extras["topic_name2"].(string)))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*kafka.Client)
				record1 := &kgo.Record{Topic: ctx.extras["topic_name1"].(string), Value: []byte("Hello Kafka!")}
				record2 := &kgo.Record{Topic: ctx.extras["topic_name2"].(string), Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record1, record2).FirstErr(), "record had a produce error while synchronously producing")
			},
			validation: validateProtocolConnection(&protocols.Stack{
				Application: protocols.Kafka,
			}),
			teardown: kafkaTeardown,
		},
	}

	for produceVersion := kafka.MinSupportedProduceRequestApiVersion; produceVersion <= kafka.MaxSupportedProduceRequestApiVersion; produceVersion++ {
		currentTest := getProduceVersionTest(int16(produceVersion), targetAddress, serverAddress, getTopicName(), defaultDialer)
		currentTest.teardown = kafkaTeardown
		tests = append(tests, currentTest)
	}

	for fetchVersion := kafka.MinSupportedFetchRequestApiVersion; fetchVersion <= kafka.MaxSupportedFetchRequestApiVersion; fetchVersion++ {
		currentTest := getFetchVersionTest(int16(fetchVersion), targetAddress, serverAddress, getTopicName(), defaultDialer)
		currentTest.teardown = kafkaTeardown
		tests = append(tests, currentTest)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInnerWithProtocolCleanup(t, tt, tr, protocols.Kafka)
		})
	}
}
