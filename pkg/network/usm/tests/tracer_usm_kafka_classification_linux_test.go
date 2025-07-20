// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package tests

import (
	"context"
	"crypto/tls"
	"fmt"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kversion"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
)

func buildProduceVersionTest(name string, version *kversion.Versions, tContext testContext, dialFn func(context.Context, string, string) (net.Conn, error), expectedStack *protocols.Stack) protocolClassificationAttributes {
	return protocolClassificationAttributes{
		name:    name,
		context: tContext,
		preTracerSetup: func(t *testing.T, ctx testContext) {
			produceClient, err := kafka.NewClient(kafka.Options{
				ServerAddress: ctx.targetAddress,
				DialFn:        dialFn,
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
		validation: validateProtocolConnection(expectedStack),
	}
}

func buildFetchVersionTest(name string, version *kversion.Versions, tContext testContext, dialFn func(context.Context, string, string) (net.Conn, error), expectedStack *protocols.Stack) protocolClassificationAttributes {
	return protocolClassificationAttributes{
		name:    name,
		context: tContext,
		preTracerSetup: func(t *testing.T, ctx testContext) {
			produceClient, err := kafka.NewClient(kafka.Options{
				ServerAddress: ctx.targetAddress,
				DialFn:        dialFn,
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
				DialFn:        dialFn,
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
		validation: validateProtocolConnection(expectedStack),
	}
}

func testKafkaProtocolClassificationInner(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string, withTLS bool) {
	const topicName = "franz-kafka"
	testIndex := 0
	// Kafka does not allow us to delete topic, but to mark them for deletion, so we have to generate a unique topic
	// per a test.
	getTopicName := func() string {
		testIndex++
		return fmt.Sprintf("%s-%d", topicName, testIndex)
	}

	// Configure plain/TLS
	skippers := []func(*testing.T, testContext){
		skipIfUsingNAT,
	}
	expectedStack := &protocols.Stack{Application: protocols.Kafka}
	if withTLS {
		skippers = append(skippers, skipIfGoTLSNotSupported)
		expectedStack.Encryption = protocols.TLS
	}

	skipFunc := composeSkips(skippers...)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		targetAddress: targetHost,
	})

	var baseDialer = &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}
	// Configure the dial function based on whether TLS is enabled or not
	dialFn := baseDialer.DialContext
	if withTLS {
		dialFn = (&tls.Dialer{
			NetDialer: baseDialer,
			Config: &tls.Config{
				InsecureSkipVerify: true,
			},
		}).DialContext
	}

	kafkaTeardown := func(_ *testing.T, ctx testContext) {
		for key, val := range ctx.extras {
			if strings.HasSuffix(key, "client") {
				client := val.(*kafka.Client)
				client.Client.Close()
			}
		}
	}

	require.NoError(t, kafka.RunServer(t, serverHost))

	tests := []protocolClassificationAttributes{
		{
			name: "connect",
			context: testContext{
				targetAddress: net.JoinHostPort(targetHost, kafka.GetPort(1.0, withTLS)),
				serverAddress: net.JoinHostPort(serverHost, kafka.GetPort(1.0, withTLS)),
				extras:        make(map[string]interface{}),
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        dialFn,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(kversion.V1_0_0())},
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
				targetAddress: net.JoinHostPort(targetHost, kafka.GetPort(1.0, withTLS)),
				serverAddress: net.JoinHostPort(serverHost, kafka.GetPort(1.0, withTLS)),
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        dialFn,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(kversion.V1_0_0())},
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
				targetAddress: net.JoinHostPort(targetHost, kafka.GetPort(1.0, withTLS)),
				serverAddress: net.JoinHostPort(serverHost, kafka.GetPort(1.0, withTLS)),
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        dialFn,
					CustomOptions: []kgo.Opt{kgo.ClientID(""), kgo.MaxVersions(kversion.V1_0_0())},
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
				targetAddress: net.JoinHostPort(targetHost, kafka.GetPort(1.0, withTLS)),
				serverAddress: net.JoinHostPort(serverHost, kafka.GetPort(1.0, withTLS)),
				extras: map[string]interface{}{
					"topic_name1": getTopicName(),
					"topic_name2": getTopicName(),
				},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        dialFn,
					CustomOptions: []kgo.Opt{kgo.ClientID(""), kgo.MaxVersions(kversion.V1_0_0())},
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

	// Generate tests for all support Produce versions
	for produceVersion := kafka.ClassificationMinSupportedProduceRequestApiVersion; produceVersion <= kafka.ClassificationMaxSupportedProduceRequestApiVersion; produceVersion++ {
		// Default to kafka v4
		versionNum := float32(4.0)
		version := kversion.V4_0_0()

		// on older versions of kafka, test against old kafka server
		if produceVersion < 8 {
			versionNum = 3.8
			version = kversion.V3_8_0()
		}
		require.LessOrEqual(t, int16(produceVersion), lo.Must(version.LookupMaxKeyVersion(produceAPIKey)), "produce version unsupported by kafka lib")
		version.SetMaxKeyVersion(produceAPIKey, int16(produceVersion))

		fmt.Println(fmt.Sprintf("produce v%d", produceVersion), net.JoinHostPort(targetHost, kafka.GetPort(versionNum, withTLS)))

		currentTest := buildProduceVersionTest(
			fmt.Sprintf("produce v%d", produceVersion),
			version,
			testContext{
				targetAddress: net.JoinHostPort(targetHost, kafka.GetPort(versionNum, withTLS)),
				serverAddress: net.JoinHostPort(serverHost, kafka.GetPort(versionNum, withTLS)),
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			dialFn,
			expectedStack,
		)
		currentTest.teardown = kafkaTeardown
		tests = append(tests, currentTest)
	}

	// Generate tests for all support Fetch versions
	for fetchVersion := kafka.ClassificationMinSupportedFetchRequestApiVersion; fetchVersion <= kafka.ClassificationMaxSupportedFetchRequestApiVersion; fetchVersion++ {
		// Default to kafka v4
		versionNum := float32(4.0)
		version := kversion.V4_0_0()

		// on older versions of kafka, test against old kafka server
		if fetchVersion < 8 {
			// The lib version has to be rolled-back from 4.0 because they dropped support for old versions of produce and fetch
			versionNum = 3.8
			version = kversion.V3_8_0()
		}
		require.LessOrEqual(t, int16(fetchVersion), lo.Must(version.LookupMaxKeyVersion(fetchAPIKey)), "fetch version unsupported by kafka lib")
		version.SetMaxKeyVersion(fetchAPIKey, int16(fetchVersion))

		currentTest := buildFetchVersionTest(
			fmt.Sprintf("fetch v%d", fetchVersion),
			version,
			testContext{
				targetAddress: net.JoinHostPort(targetHost, kafka.GetPort(versionNum, withTLS)),
				serverAddress: net.JoinHostPort(serverHost, kafka.GetPort(versionNum, withTLS)),
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			dialFn,
			expectedStack,
		)
		currentTest.teardown = kafkaTeardown
		tests = append(tests, currentTest)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInnerWithProtocolCleanup(t, tt, tr, protocols.Kafka)
		})
	}
}

func testKafkaProtocolClassification(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	testKafkaProtocolClassificationInner(t, tr, clientHost, targetHost, serverHost, protocolsUtils.TLSDisabled)
}

func testKafkaProtocolClassificationTLS(t *testing.T, tr *tracer.Tracer, clientHost, targetHost, serverHost string) {
	testKafkaProtocolClassificationInner(t, tr, clientHost, targetHost, serverHost, protocolsUtils.TLSEnabled)
}
