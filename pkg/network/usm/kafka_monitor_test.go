// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"io"
	"net"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kversion"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	kafkaPort = "9092"
)

// testContext shares the context of a given test.
// It contains common variable used by all tests, and allows extending the context dynamically by setting more
// attributes to the `extras` map.
type testContext struct {
	// The address of the server to listen on.
	serverAddress string
	// The port to listen on.
	serverPort string
	// The address for the client to communicate with.
	targetAddress string
	// A dynamic map that allows extending the context easily between phases of the test.
	extras map[string]interface{}
}

// kafkaParsingTestAttributes holds all attributes a single kafka parsing test should have.
type kafkaParsingTestAttributes struct {
	// The name of the test.
	name string
	// Specific test context, allows to share states among different phases of the test.
	context testContext
	// The test body
	testBody func(t *testing.T, ctx testContext, monitor *Monitor)
	// Cleaning test resources if needed.
	teardown func(t *testing.T, ctx testContext)
	// Configuration for the monitor object
	configuration func() *config.Config
}

type kafkaParsingValidation struct {
	expectedNumberOfProduceRequests int
	expectedNumberOfFetchRequests   int
	expectedApiVersionProduce       int
	expectedApiVersionFetch         int
}

func skipTestIfKernelNotSupported(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < http.MinimumKernelVersion {
		t.Skip(fmt.Sprintf("Kafka feature not available on pre %s kernels", http.MinimumKernelVersion.String()))
	}
}

func TestKafkaProtocolParsing(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "", testKafkaProtocolParsing)
}

func testKafkaProtocolParsing(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	clientHost := "localhost"
	targetHost := "127.0.0.1"
	serverHost := "127.0.0.1"

	testIndex := 0
	// Kafka does not allow us to delete topic, but to mark them for deletion, so we have to generate a unique topic
	// per a test.
	getTopicName := func() string {
		testIndex++
		return fmt.Sprintf("%s-%d", "franz-kafka", testIndex)
	}

	kafkaTeardown := func(t *testing.T, ctx testContext) {
		if _, ok := ctx.extras["client"]; !ok {
			return
		}
		if client, ok := ctx.extras["client"].(*kafka.Client); ok {
			defer client.Client.Close()
			for k, value := range ctx.extras {
				if strings.HasPrefix(k, "topic_name") {
					_ = client.DeleteTopic(value.(string))
				}
			}
		}
	}

	serverAddress := net.JoinHostPort(serverHost, kafkaPort)
	targetAddress := net.JoinHostPort(targetHost, kafkaPort)
	require.NoError(t, kafka.RunServer(t, serverHost, kafkaPort))

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	tests := []kafkaParsingTestAttributes{
		{
			name: "Sanity - produce and fetch",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			testBody: func(t *testing.T, ctx testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(kversion.V2_5_0()),
						kgo.ConsumeTopics(topicName),
					},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(topicName))

				record := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")

				fetches := client.Client.PollFetches(context.Background())
				errs := fetches.Errors()
				for _, err := range errs {
					t.Errorf("PollFetches error: %+v", err)
					t.FailNow()
				}

				// We expect 2 occurrences for each connection as we are working with a docker, so (1 produce + 1 fetch) * 2 = (4 stats)
				kafkaStats := getAndValidateKafkaStats(t, monitor, 4)

				// kgo client is sending an extra fetch request before running the test, so double the expected fetch request
				validateProduceFetchCount(t, kafkaStats, topicName, kafkaParsingValidation{
					expectedNumberOfProduceRequests: 2,
					expectedNumberOfFetchRequests:   4,
					expectedApiVersionProduce:       8,
					expectedApiVersionFetch:         11,
				})
			},
			teardown:      kafkaTeardown,
			configuration: getDefaultTestConfiguration,
		},
		{
			name: "TestProduceClientIdEmptyString",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			testBody: func(t *testing.T, ctx testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(kversion.V1_0_0()),
						kgo.ClientID(""),
					},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(topicName))

				record := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")

				// We expect 2 occurrences for each connection as we are working with a docker, so (1 produce) * 2 = (2 stats)
				kafkaStats := getAndValidateKafkaStats(t, monitor, 2)

				validateProduceFetchCount(t, kafkaStats, topicName, kafkaParsingValidation{
					expectedNumberOfProduceRequests: 2,
					expectedNumberOfFetchRequests:   0,
					expectedApiVersionProduce:       5,
					expectedApiVersionFetch:         0,
				})
			},
			teardown:      kafkaTeardown,
			configuration: getDefaultTestConfiguration,
		},
		{
			name: "TestManyProduceRequests",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			testBody: func(t *testing.T, ctx testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(kversion.V2_5_0()),
						kgo.ClientID(""),
					},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(topicName))

				numberOfIterations := 1000
				for i := 1; i <= numberOfIterations; i++ {
					record := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka!")}
					ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
					require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")
					cancel()
				}

				// We expect 2 occurrences for each connection as we are working with a docker, so (1 produce) * 2 = (2 stats)
				kafkaStats := getAndValidateKafkaStats(t, monitor, 2)
				validateProduceFetchCount(t, kafkaStats, topicName, kafkaParsingValidation{
					expectedNumberOfProduceRequests: numberOfIterations * 2,
					expectedNumberOfFetchRequests:   0,
					expectedApiVersionProduce:       8,
					expectedApiVersionFetch:         0,
				})
			},
			teardown:      kafkaTeardown,
			configuration: getDefaultTestConfiguration,
		},
		{
			name: "TestHTTPAndKafka",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			testBody: func(t *testing.T, ctx testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(kversion.V2_5_0()),
						kgo.ClientID(""),
					},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(topicName))

				record := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")
				cancel()

				serverAddr := "localhost:8081"
				srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{})
				t.Cleanup(srvDoneFn)
				httpClient := nethttp.Client{}

				req, err := nethttp.NewRequest(httpMethods[0], fmt.Sprintf("http://%s/%d/request", serverAddr, nethttp.StatusOK), nil)
				require.NoError(t, err)

				httpRequestCount := 10
				for i := 0; i < httpRequestCount; i++ {
					resp, err := httpClient.Do(req)
					require.NoError(t, err)
					// Have to read the response body to ensure the client will be able to properly close the connection.
					io.ReadAll(resp.Body)
					resp.Body.Close()
				}
				srvDoneFn()

				httpOccurrences := PrintableInt(0)
				expectedKafkaRequestCount := 2
				kafkaStatsCount := PrintableInt(0)
				kafkaStats := make(map[kafka.Key]*kafka.RequestStat)
				require.Eventually(t, func() bool {
					allStats := monitor.GetProtocolStats()
					require.NotNil(t, allStats)

					httpStats, ok := allStats[protocols.HTTP]
					if ok {
						httpOccurrences.Add(countRequestOccurrences(httpStats.(map[http.Key]*http.RequestStats), req))
					}

					kafkaProtocolStats, ok := allStats[protocols.Kafka]
					// We might not have kafka stats, and it might be the expected case (to capture 0).
					if ok {
						currentStats := kafkaProtocolStats.(map[kafka.Key]*kafka.RequestStat)
						for key, stats := range currentStats {
							prevStats, ok := kafkaStats[key]
							if ok && prevStats != nil {
								prevStats.CombineWith(stats)
							} else {
								kafkaStats[key] = currentStats[key]
							}
						}
					}
					kafkaStatsCount = PrintableInt(len(kafkaStats))
					return len(kafkaStats) == expectedKafkaRequestCount && httpOccurrences.Load() == httpRequestCount
				}, time.Second*3, time.Millisecond*100, "Expected to find %d http requests (captured %v), and %d kafka requests (captured %v)", httpRequestCount, &httpOccurrences, expectedKafkaRequestCount, &kafkaStatsCount)

				// We expect 2 occurrences for each connection as we are working with a docker, so (1 produce) * 2 = (2 stats)
				validateProduceFetchCount(t, kafkaStats, topicName,
					kafkaParsingValidation{
						expectedNumberOfProduceRequests: 2,
						expectedNumberOfFetchRequests:   0,
						expectedApiVersionProduce:       8,
						expectedApiVersionFetch:         0,
					})
			},
			teardown: kafkaTeardown,
			configuration: func() *config.Config {
				cfg := config.New()
				cfg.EnableHTTPMonitoring = true
				cfg.EnableKafkaMonitoring = true
				cfg.MaxTrackedConnections = 1000
				return cfg
			},
		},
		{
			name: "TestEnableHTTPOnly",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": getTopicName(),
				},
			},
			testBody: func(t *testing.T, ctx testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(kversion.V2_5_0()),
						kgo.ClientID(""),
					},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(topicName))

				record := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")
				cancel()

				getAndValidateKafkaStats(t, monitor, 0)
			},
			teardown: kafkaTeardown,
			configuration: func() *config.Config {
				cfg := config.New()
				cfg.EnableHTTPMonitoring = true
				return cfg
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolParsingInner(t, tt, tt.configuration())
		})
	}
}

type PrintableInt int

func (i *PrintableInt) String() string {
	if i == nil {
		return "nil"
	}

	return fmt.Sprintf("%d", *i)
}

func (i *PrintableInt) Load() int {
	if i == nil {
		return 0
	}

	return int(*i)
}

func (i *PrintableInt) Add(other int) {
	*i = PrintableInt(other + i.Load())
}

func getAndValidateKafkaStats(t *testing.T, monitor *Monitor, expectedStatsCount int) map[kafka.Key]*kafka.RequestStat {
	statsCount := PrintableInt(0)
	kafkaStats := make(map[kafka.Key]*kafka.RequestStat)
	require.Eventually(t, func() bool {
		protocolStats := monitor.GetProtocolStats()
		kafkaProtocolStats, exists := protocolStats[protocols.Kafka]
		// We might not have kafka stats, and it might be the expected case (to capture 0).
		if exists {
			currentStats := kafkaProtocolStats.(map[kafka.Key]*kafka.RequestStat)
			for key, stats := range currentStats {
				prevStats, ok := kafkaStats[key]
				if ok && prevStats != nil {
					prevStats.CombineWith(stats)
				} else {
					kafkaStats[key] = currentStats[key]
				}
			}
		}
		statsCount = PrintableInt(len(kafkaStats))
		return expectedStatsCount == len(kafkaStats)
	}, time.Second*5, time.Millisecond*100, "Expected to find a %d stats, instead captured %v", expectedStatsCount, &statsCount)
	return kafkaStats
}

func validateProduceFetchCount(t *testing.T, kafkaStats map[kafka.Key]*kafka.RequestStat, topicName string, validation kafkaParsingValidation) {
	numberOfProduceRequests := 0
	numberOfFetchRequests := 0
	for kafkaKey, kafkaStat := range kafkaStats {
		require.Equal(t, topicName, kafkaKey.TopicName)
		switch kafkaKey.RequestAPIKey {
		case kafka.ProduceAPIKey:
			require.Equal(t, uint16(validation.expectedApiVersionProduce), kafkaKey.RequestVersion)
			numberOfProduceRequests += kafkaStat.Count
			break
		case kafka.FetchAPIKey:
			require.Equal(t, uint16(validation.expectedApiVersionFetch), kafkaKey.RequestVersion)
			numberOfFetchRequests += kafkaStat.Count
			break
		default:
			require.FailNow(t, "Expecting only produce or fetch kafka requests")
		}
	}
	require.Equal(t, validation.expectedNumberOfProduceRequests, numberOfProduceRequests,
		"Expected %d produce requests but got %d", validation.expectedNumberOfProduceRequests, numberOfProduceRequests)
	require.Equal(t, validation.expectedNumberOfFetchRequests, numberOfFetchRequests,
		"Expected %d produce requests but got %d", validation.expectedNumberOfFetchRequests, numberOfFetchRequests)
}

func testProtocolParsingInner(t *testing.T, params kafkaParsingTestAttributes, cfg *config.Config) {
	if params.teardown != nil {
		t.Cleanup(func() {
			params.teardown(t, params.context)
		})
	}
	monitor := newKafkaMonitor(t, cfg)
	params.testBody(t, params.context, monitor)
}

func getDefaultTestConfiguration() *config.Config {
	cfg := config.New()
	cfg.EnableKafkaMonitoring = true
	cfg.MaxTrackedConnections = 1000
	return cfg
}

func newKafkaMonitor(t *testing.T, cfg *config.Config) *Monitor {
	monitor, err := NewMonitor(cfg, nil, nil, nil)
	skipIfNotSupported(t, err)
	require.NoError(t, err)
	t.Cleanup(func() {
		monitor.Stop()
	})

	err = monitor.Start()
	require.NoError(t, err)
	return monitor
}

// This test will help us identify if there is any verifier problems while loading the Kafka binary in the CI environment
func TestLoadKafkaBinary(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		t.Run("debug", func(t *testing.T) {
			loadKafkaBinary(t, true)
		})
		t.Run("release", func(t *testing.T) {
			loadKafkaBinary(t, false)
		})
	})
}

func loadKafkaBinary(t *testing.T, debug bool) {
	cfg := config.New()
	// We don't have a way of enabling kafka without http at the moment
	cfg.EnableKafkaMonitoring = true
	cfg.MaxTrackedConnections = 1000
	cfg.BPFDebug = debug

	newKafkaMonitor(t, cfg)
}
