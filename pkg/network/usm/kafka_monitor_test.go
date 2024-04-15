// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"testing"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/franz-go/pkg/kversion"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/google/uuid"
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
	expectedAPIVersionProduce       int
	expectedAPIVersionFetch         int
}

func skipTestIfKernelNotSupported(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < http.MinimumKernelVersion {
		t.Skipf("Kafka feature not available on pre %s kernels", http.MinimumKernelVersion.String())
	}
}

type KafkaProtocolParsingSuite struct {
	suite.Suite
}

func (s *KafkaProtocolParsingSuite) getTopicName() string {
	// Use unique names for topics to avoid having tests cases
	// affect each other due to, for example, returning older records.
	return fmt.Sprintf("%s-%s", "franz-kafka", uuid.New().String())
}

func TestKafkaProtocolParsing(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	serverHost := "127.0.0.1"
	require.NoError(t, kafka.RunServer(t, serverHost, kafkaPort))

	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		suite.Run(t, new(KafkaProtocolParsingSuite))
	})
}

func (s *KafkaProtocolParsingSuite) TestKafkaProtocolParsing() {
	t := s.T()

	clientHost := "localhost"
	targetHost := "127.0.0.1"
	serverHost := "127.0.0.1"

	kafkaTeardown := func(t *testing.T, ctx testContext) {
		if _, ok := ctx.extras["client"]; !ok {
			return
		}
		if client, ok := ctx.extras["client"].(*kafka.Client); ok {
			defer client.Client.Close()
		}
	}

	serverAddress := net.JoinHostPort(serverHost, kafkaPort)
	targetAddress := net.JoinHostPort(targetHost, kafkaPort)

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
					"topic_name": s.getTopicName(),
				},
			},
			testBody: func(t *testing.T, ctx testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(kversion.V2_5_0()),
						kgo.RecordPartitioner(kgo.ManualPartitioner()),
						kgo.ClientID("xk6-kafka_linux_amd64@foobar (github.com/segmentio/kafka-go)"),
					},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(topicName))

				record := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")

				req := kmsg.NewFetchRequest()
				topic := kmsg.NewFetchRequestTopic()
				topic.Topic = topicName
				partition := kmsg.NewFetchRequestTopicPartition()
				partition.PartitionMaxBytes = 1024
				topic.Partitions = append(topic.Partitions, partition)
				req.Topics = append(req.Topics, topic)

				_, err = req.RequestWith(ctxTimeout, client.Client)
				require.NoError(t, err)

				// We expect 2 occurrences for each connection as we are working with a docker, so (1 produce + 1 fetch) * 2 = (4 stats)
				kafkaStats := getAndValidateKafkaStats(t, monitor, 4)

				validateProduceFetchCount(t, kafkaStats, topicName, kafkaParsingValidation{
					expectedNumberOfProduceRequests: 2,
					expectedNumberOfFetchRequests:   2,
					expectedAPIVersionProduce:       8,
					expectedAPIVersionFetch:         11,
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
					"topic_name": s.getTopicName(),
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
					expectedAPIVersionProduce:       5,
					expectedAPIVersionFetch:         0,
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
					"topic_name": s.getTopicName(),
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
					expectedAPIVersionProduce:       8,
					expectedAPIVersionFetch:         0,
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
					"topic_name": s.getTopicName(),
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
					io.Copy(io.Discard, resp.Body)
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
						expectedAPIVersionProduce:       8,
						expectedAPIVersionFetch:         0,
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
					"topic_name": s.getTopicName(),
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
				cfg.MaxTrackedConnections = 1000
				return cfg
			},
		},
		{
			name: "Multiple records within the same produce requests",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": s.getTopicName(),
				},
			},
			testBody: func(t *testing.T, ctx testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(kversion.V2_5_0()),
						kgo.RecordPartitioner(kgo.ManualPartitioner()),
					},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(topicName))

				record1 := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka!")}
				record2 := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka again!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record1, record2).FirstErr(), "record had a produce error while synchronously producing")

				req := kmsg.NewFetchRequest()
				topic := kmsg.NewFetchRequestTopic()
				topic.Topic = topicName
				partition := kmsg.NewFetchRequestTopicPartition()
				partition.PartitionMaxBytes = 1024
				topic.Partitions = append(topic.Partitions, partition)
				req.Topics = append(req.Topics, topic)

				_, err = req.RequestWith(ctxTimeout, client.Client)
				require.NoError(t, err)

				// We expect 2 occurrences for each connection as we are working with a docker, so (1 produce) * 2 = (2 stats)
				kafkaStats := getAndValidateKafkaStats(t, monitor, 2*2)

				validateProduceFetchCount(t, kafkaStats, topicName, kafkaParsingValidation{
					expectedNumberOfProduceRequests: 2 * 2,
					expectedNumberOfFetchRequests:   2 * 2,
					expectedAPIVersionProduce:       8,
					expectedAPIVersionFetch:         11,
				})
			},
			teardown:      kafkaTeardown,
			configuration: getDefaultTestConfiguration,
		},
		{
			name: "Multiple records with and without batching",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras: map[string]interface{}{
					"topic_name": s.getTopicName(),
				},
			},
			testBody: func(t *testing.T, ctx testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(kversion.V2_5_0()),
						kgo.RecordPartitioner(kgo.ManualPartitioner()),
					},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(topicName))

				record1 := &kgo.Record{Topic: topicName, Partition: 1, Value: []byte("Hello Kafka!")}
				record2 := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka again!")}

				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record1).FirstErr())
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record2).FirstErr())
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record1, record1).FirstErr())
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record1).FirstErr())

				var batch []*kgo.Record
				for i := 0; i < 25; i++ {
					batch = append(batch, record1)
				}
				for i := 0; i < 25; i++ {
					require.NoError(t, client.Client.ProduceSync(ctxTimeout, batch...).FirstErr())
				}

				req := kmsg.NewFetchRequest()
				topic := kmsg.NewFetchRequestTopic()
				topic.Topic = topicName
				partition := kmsg.NewFetchRequestTopicPartition()
				partition.PartitionMaxBytes = 1024 * 1024
				partition1 := kmsg.NewFetchRequestTopicPartition()
				partition1.Partition = 1
				partition1.PartitionMaxBytes = 1024 * 1024
				topic.Partitions = append(topic.Partitions, partition, partition1)
				req.Topics = append(req.Topics, topic)

				_, err = req.RequestWith(ctxTimeout, client.Client)
				require.NoError(t, err)

				// We expect 2 occurrences for each connection as we are working with a docker, so (1 produce) * 2 = (2 stats)
				kafkaStats := getAndValidateKafkaStats(t, monitor, 2*2)

				validateProduceFetchCount(t, kafkaStats, topicName, kafkaParsingValidation{
					expectedNumberOfProduceRequests: (5 + 25*25) * 2,
					expectedNumberOfFetchRequests:   (5 + 25*25) * 2,
					expectedAPIVersionProduce:       8,
					expectedAPIVersionFetch:         11,
				})
			},
			teardown:      kafkaTeardown,
			configuration: getDefaultTestConfiguration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolParsingInner(t, tt, tt.configuration())
		})
	}
}

func generateFetchRequest(topic string) kmsg.FetchRequest {
	req := kmsg.NewFetchRequest()
	req.SetVersion(11)
	reqTopic := kmsg.NewFetchRequestTopic()
	reqTopic.Topic = topic
	partition := kmsg.NewFetchRequestTopicPartition()
	partition.PartitionMaxBytes = 1024
	reqTopic.Partitions = append(reqTopic.Partitions, partition)
	req.Topics = append(req.Topics, reqTopic)
	return req
}

func makeRecord() kmsg.Record {
	var tmp []byte
	record := kmsg.NewRecord()
	record.Value = []byte("Hello Kafka!")
	tmp = record.AppendTo(make([]byte, 0))
	// 1 is the length of varint encoded 0
	record.Length = int32(len(tmp) - 1)
	return record
}

func makeRecordBatch(records ...kmsg.Record) kmsg.RecordBatch {
	recordBatch := kmsg.NewRecordBatch()
	recordBatch.Magic = 2

	recordBatch.NumRecords = int32(len(records))
	for _, record := range records {
		recordBatch.Records = record.AppendTo(recordBatch.Records)
	}

	tmp := recordBatch.AppendTo(make([]byte, 0))
	// Length excludes sizeof(FirstOffset + Length)
	recordBatch.Length = int32(len(tmp) - 12)

	return recordBatch
}

func makeFetchResponseTopicPartition(recordBatches ...kmsg.RecordBatch) kmsg.FetchResponseTopicPartition {
	respParition := kmsg.NewFetchResponseTopicPartition()

	for _, recordBatch := range recordBatches {
		respParition.RecordBatches = recordBatch.AppendTo(respParition.RecordBatches)
	}

	return respParition
}

func makeFetchResponseTopic(topic string, partitions ...kmsg.FetchResponseTopicPartition) kmsg.FetchResponseTopic {
	respTopic := kmsg.NewFetchResponseTopic()
	respTopic.Topic = topic
	respTopic.Partitions = append(respTopic.Partitions, partitions...)
	return respTopic
}

func makeFetchResponse(topics ...kmsg.FetchResponseTopic) kmsg.FetchResponse {
	resp := kmsg.NewFetchResponse()
	resp.SetVersion(11)
	resp.Topics = append(resp.Topics, topics...)
	return resp
}

func appendUint32(dst []byte, u uint32) []byte {
	return append(dst, byte(u>>24), byte(u>>16), byte(u>>8), byte(u))
}

// kmsg doesn't have a ResponseFormatter so we need to add the length
// and the correlation Id ourselves.
func appendResponse(dst []byte, response kmsg.FetchResponse, correlationId uint32) []byte {
	var data []byte
	data = response.AppendTo(data)

	// Length excludes the field itself
	dst = appendUint32(dst, uint32(len(data)+4))
	dst = appendUint32(dst, correlationId)
	dst = append(dst, data...)

	return dst
}

type Message struct {
	request  []byte
	response []byte
}

func runCannedTransaction(t *testing.T, msgs []Message) {
	address := "127.0.0.1:9092"
	listener, err := net.Listen("tcp", address)
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		require.NoError(t, err)
		defer conn.Close()

		reader := bufio.NewReader(conn)

		for _, msg := range msgs {
			if len(msg.request) > 0 {
				_, err := io.ReadFull(reader, msg.request)
				require.NoError(t, err)
			}

			if len(msg.response) > 0 {
				conn.Write(msg.response)
			}
		}
	}()

	conn, err := net.Dial("tcp", address)
	require.NoError(t, err)
	defer conn.Close()

	reader := bufio.NewReader(conn)
	for _, msg := range msgs {
		if len(msg.request) > 0 {
			// Note that the net package sets TCP_NODELAY by default,
			// so this will send out each msg individually, which
			// is which we want to test split segment handling.
			conn.Write(msg.request)
		}

		if len(msg.response) > 0 {
			_, err := io.ReadFull(reader, msg.response)
			require.NoError(t, err)
		}
	}
}

func appendMessages(messages []Message, correlationId int, req kmsg.FetchRequest, resp kmsg.FetchResponse) []Message {
	formatter := kmsg.NewRequestFormatter(kmsg.FormatterClientID("kgo"))
	data := formatter.AppendRequest(make([]byte, 0), &req, int32(correlationId))
	respData := appendResponse(make([]byte, 0), resp, uint32(correlationId))

	return append(messages,
		Message{request: data},
		Message{response: respData},
	)
}

func TestKafkaFetchRaw(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	topic := "test-topic"
	tests := []struct {
		name              string
		buildResponse     func() kmsg.FetchResponse
		numFetchedRecords int
	}{
		{
			name: "basic",
			buildResponse: func() kmsg.FetchResponse {
				record := makeRecord()
				var records []kmsg.Record
				for i := 0; i < 5; i++ {
					records = append(records, record)
				}

				recordBatch := makeRecordBatch(records...)
				var batches []kmsg.RecordBatch
				for i := 0; i < 4; i++ {
					batches = append(batches, recordBatch)
				}

				partition := makeFetchResponseTopicPartition(batches...)
				var partitions []kmsg.FetchResponseTopicPartition
				for i := 0; i < 3; i++ {
					partitions = append(partitions, partition)
				}

				return makeFetchResponse(makeFetchResponseTopic(topic, partitions...))
			},
			numFetchedRecords: 5 * 4 * 3,
		},
		{
			name: "aborted transactions",
			buildResponse: func() kmsg.FetchResponse {
				record := makeRecord()
				partition := makeFetchResponseTopicPartition(makeRecordBatch(record, record))
				aborted := kmsg.NewFetchResponseTopicPartitionAbortedTransaction()

				for i := 0; i < 10; i++ {
					partition.AbortedTransactions = append(partition.AbortedTransactions, aborted)
				}

				return makeFetchResponse(makeFetchResponseTopic(topic, partition))
			},
			numFetchedRecords: 2,
		},
		{
			name: "partial record batch",
			buildResponse: func() kmsg.FetchResponse {
				record := makeRecord()
				recordBatch := makeRecordBatch(record, record, record)
				partition := makeFetchResponseTopicPartition(recordBatch)

				// Partial record batch, aka "Truncated Content" in Wireshark.  See
				// comment near FetchResponseTopicPartition.RecordBatch in kmsg.
				tmp := recordBatch.AppendTo(make([]byte, 0))
				partition.RecordBatches = append(partition.RecordBatches, tmp[:len(tmp)-1]...)

				return makeFetchResponse(makeFetchResponseTopic(topic, partition))
			},
			numFetchedRecords: 3,
		},
	}

	req := generateFetchRequest(topic)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := tt.buildResponse()
			var msgs []Message
			msgs = appendMessages(msgs, 99, req, resp)

			monitor := newKafkaMonitor(t, getDefaultTestConfiguration())
			runCannedTransaction(t, msgs)
			kafkaStats := getAndValidateKafkaStats(t, monitor, 1)

			validateProduceFetchCount(t, kafkaStats, topic, kafkaParsingValidation{
				expectedNumberOfFetchRequests: tt.numFetchedRecords,
				expectedAPIVersionFetch:       11,
			})
		})

		name := fmt.Sprintf("split/%s", tt.name)
		t.Run(name, func(t *testing.T) {
			resp := tt.buildResponse()

			formatter := kmsg.NewRequestFormatter(kmsg.FormatterClientID("kgo"))

			var msgs []Message
			splitIdx := 0
			for splitIdx = 0; splitIdx < 1000; splitIdx++ {
				reqData := formatter.AppendRequest(make([]byte, 0), &req, int32(splitIdx))
				respData := appendResponse(make([]byte, 0), resp, uint32(splitIdx))

				// There is an assumption in the code that the first segment contains the data
				// up to and including the number of paritions.  This size is 38 bytes with
				// the topic name test-topic and API version 11.
				minSegSize := 38
				require.Equal(t, topic, "test-topic")
				require.Equal(t, int(req.GetVersion()), 11)

				segSize := min(minSegSize+splitIdx, len(respData))
				if segSize >= len(respData) {
					break
				}

				msgs = append(msgs, Message{request: reqData})
				msgs = append(msgs, Message{response: respData[0:segSize]})

				if segSize+8 >= len(respData) {
					msgs = append(msgs, Message{response: respData[segSize:]})
				} else {
					// Three segments tests other code paths than two, for example
					// it will fail if the tcp_seq is not updated in the response
					// parsing continuation path.
					msgs = append(msgs, Message{response: respData[segSize : segSize+8]})
					msgs = append(msgs, Message{response: respData[segSize+8:]})
				}

			}

			monitor := newKafkaMonitor(t, getDefaultTestConfiguration())
			runCannedTransaction(t, msgs)
			kafkaStats := getAndValidateKafkaStats(t, monitor, 1)

			validateProduceFetchCount(t, kafkaStats, topic, kafkaParsingValidation{
				expectedNumberOfFetchRequests: tt.numFetchedRecords * splitIdx,
				expectedAPIVersionFetch:       11,
			})
		})
	}
}

func TestKafkaInFlightMapCleaner(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	cfg := getDefaultTestConfiguration()
	cfg.HTTPMapCleanerInterval = 5 * time.Second
	cfg.HTTPIdleConnectionTTL = time.Second
	monitor := newKafkaMonitor(t, cfg)
	ebpfNow, err := ddebpf.NowNanoseconds()
	require.NoError(t, err)
	inFlightMap, _, err := monitor.ebpfProgram.GetMap("kafka_in_flight")
	require.NoError(t, err)
	key := kafka.KafkaTransactionKey{
		Id: 99,
	}
	val := kafka.KafkaTransaction{
		Request_started: uint64(ebpfNow - (time.Second * 3).Nanoseconds()),
		Request_api_key: 55,
	}
	require.NoError(t, inFlightMap.Update(unsafe.Pointer(&key), unsafe.Pointer(&val), ebpf.UpdateAny))

	var newVal kafka.KafkaTransaction
	require.NoError(t, inFlightMap.Lookup(unsafe.Pointer(&key), unsafe.Pointer(&newVal)))
	require.Equal(t, val, newVal)

	require.Eventually(t, func() bool {
		err := inFlightMap.Lookup(unsafe.Pointer(&key), unsafe.Pointer(&newVal))
		return errors.Is(err, ebpf.ErrKeyNotExist)
	}, 3*cfg.HTTPMapCleanerInterval, time.Millisecond*100)
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
			require.Equal(t, uint16(validation.expectedAPIVersionProduce), kafkaKey.RequestVersion)
			numberOfProduceRequests += kafkaStat.Count
		case kafka.FetchAPIKey:
			require.Equal(t, uint16(validation.expectedAPIVersionFetch), kafkaKey.RequestVersion)
			numberOfFetchRequests += kafkaStat.Count
		default:
			require.FailNow(t, "Expecting only produce or fetch kafka requests")
		}
	}
	require.Equal(t, validation.expectedNumberOfProduceRequests, numberOfProduceRequests,
		"Expected %d produce requests but got %d", validation.expectedNumberOfProduceRequests, numberOfProduceRequests)
	require.Equal(t, validation.expectedNumberOfFetchRequests, numberOfFetchRequests,
		"Expected %d fetch requests but got %d", validation.expectedNumberOfFetchRequests, numberOfFetchRequests)
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
	monitor, err := NewMonitor(cfg, nil)
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
