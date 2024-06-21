// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/franz-go/pkg/kversion"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	gotlsutils "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/proxy"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	kafkaPort    = "9092"
	kafkaTLSPort = "9093"
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
	// Clients that should be torn down at the end of the test
	clients []*kafka.Client
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
	testBody func(t *testing.T, ctx *testContext, monitor *Monitor)
}

type kafkaParsingValidation struct {
	expectedNumberOfProduceRequests int
	expectedNumberOfFetchRequests   int
	expectedAPIVersionProduce       int
	expectedAPIVersionFetch         int
	tlsEnabled                      bool
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

	var versions []*kversion.Versions
	versions = append(versions, kversion.V2_5_0())

	fetch12 := kversion.V3_4_0()
	fetch12.SetMaxKeyVersion(kafka.ProduceAPIKey, 8)
	fetch12.SetMaxKeyVersion(kafka.FetchAPIKey, 12)
	versions = append(versions, fetch12)

	versionName := func(version *kversion.Versions) string {
		produce, found := version.LookupMaxKeyVersion(kafka.ProduceAPIKey)
		require.True(t, found)
		fetch, found := version.LookupMaxKeyVersion(kafka.FetchAPIKey)
		require.True(t, found)
		return fmt.Sprintf("produce%d_fetch%d", produce, fetch)
	}

	t.Run("without TLS", func(t *testing.T) {
		for _, version := range versions {
			t.Run(versionName(version), func(t *testing.T) {
				s.testKafkaProtocolParsing(t, false, version)
			})
		}
	})

	t.Run("with TLS", func(t *testing.T) {
		if !gotlsutils.GoTLSSupported(t, config.New()) {
			t.Skip("GoTLS not supported for this setup")
		}
		for _, version := range versions {
			t.Run(versionName(version), func(t *testing.T) {
				s.testKafkaProtocolParsing(t, true, version)
			})
		}
	})
}

func (s *KafkaProtocolParsingSuite) testKafkaProtocolParsing(t *testing.T, tls bool, version *kversion.Versions) {
	const (
		targetHost = "127.0.0.1"
		serverHost = "127.0.0.1"
		unixPath   = "/tmp/transparent.sock"
	)

	port := kafkaPort
	if tls {
		port = kafkaTLSPort
	}

	serverAddress := net.JoinHostPort(serverHost, port)
	targetAddress := net.JoinHostPort(targetHost, port)

	dialFn := func(ctx context.Context, network, address string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", unixPath)
	}

	// With non-TLS, we need to double the stats since we use Docker and the
	// packets are seen twice. This is not needed in the TLS case since there
	// the data comes from uprobes on the binary.
	fixCount := func(count int) int {
		if tls {
			return count
		}

		return count * 2
	}

	tmp, found := version.LookupMaxKeyVersion(kafka.FetchAPIKey)
	require.True(t, found)
	expectedAPIVersionFetch := int(tmp)

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
			testBody: func(t *testing.T, ctx *testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        dialFn,

					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(version),
						kgo.RecordPartitioner(kgo.ManualPartitioner()),
						kgo.ClientID("xk6-kafka_linux_amd64@foobar (github.com/segmentio/kafka-go)"),
					},
				})
				require.NoError(t, err)
				ctx.clients = append(ctx.clients, client)
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

				getAndValidateKafkaStats(t, monitor, fixCount(2), topicName, kafkaParsingValidation{
					expectedNumberOfProduceRequests: fixCount(1),
					expectedNumberOfFetchRequests:   fixCount(1),
					expectedAPIVersionProduce:       8,
					expectedAPIVersionFetch:         expectedAPIVersionFetch,
					tlsEnabled:                      tls,
				})
			},
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
			testBody: func(t *testing.T, ctx *testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        dialFn,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(kversion.V1_0_0()),
						kgo.ClientID(""),
					},
				})
				require.NoError(t, err)
				ctx.clients = append(ctx.clients, client)
				require.NoError(t, client.CreateTopic(topicName))

				record := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")

				getAndValidateKafkaStats(t, monitor, fixCount(1), topicName, kafkaParsingValidation{
					expectedNumberOfProduceRequests: fixCount(1),
					expectedNumberOfFetchRequests:   0,
					expectedAPIVersionProduce:       5,
					expectedAPIVersionFetch:         0,
					tlsEnabled:                      tls,
				})
			},
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
			testBody: func(t *testing.T, ctx *testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        dialFn,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(version),
						kgo.ClientID(""),
					},
				})
				require.NoError(t, err)
				ctx.clients = append(ctx.clients, client)
				require.NoError(t, client.CreateTopic(topicName))

				numberOfIterations := 1000
				for i := 1; i <= numberOfIterations; i++ {
					record := &kgo.Record{Topic: topicName, Value: []byte("Hello Kafka!")}
					ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
					require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")
					cancel()
				}

				getAndValidateKafkaStats(t, monitor, fixCount(1), topicName, kafkaParsingValidation{
					expectedNumberOfProduceRequests: fixCount(numberOfIterations),
					expectedNumberOfFetchRequests:   0,
					expectedAPIVersionProduce:       8,
					expectedAPIVersionFetch:         0,
					tlsEnabled:                      tls,
				})
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
			testBody: func(t *testing.T, ctx *testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        dialFn,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(version),
						kgo.RecordPartitioner(kgo.ManualPartitioner()),
					},
				})
				require.NoError(t, err)
				ctx.clients = append(ctx.clients, client)
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

				getAndValidateKafkaStats(t, monitor, fixCount(2), topicName, kafkaParsingValidation{
					expectedNumberOfProduceRequests: fixCount(2),
					expectedNumberOfFetchRequests:   fixCount(2),
					expectedAPIVersionProduce:       8,
					expectedAPIVersionFetch:         expectedAPIVersionFetch,
					tlsEnabled:                      tls,
				})
			},
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
			testBody: func(t *testing.T, ctx *testContext, monitor *Monitor) {
				topicName := ctx.extras["topic_name"].(string)
				client, err := kafka.NewClient(kafka.Options{
					ServerAddress: ctx.targetAddress,
					DialFn:        dialFn,
					CustomOptions: []kgo.Opt{
						kgo.MaxVersions(version),
						kgo.RecordPartitioner(kgo.ManualPartitioner()),
					},
				})
				require.NoError(t, err)
				ctx.clients = append(ctx.clients, client)
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
				for i := 0; i < 2; i++ {
					batch = append(batch, record1)
				}
				for i := 0; i < 2; i++ {
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

				getAndValidateKafkaStats(t, monitor, fixCount(2), topicName, kafkaParsingValidation{
					expectedNumberOfProduceRequests: fixCount(5 + 2*2),
					expectedNumberOfFetchRequests:   fixCount(5 + 2*2),
					expectedAPIVersionProduce:       8,
					expectedAPIVersionFetch:         expectedAPIVersionFetch,
					tlsEnabled:                      tls,
				})
			},
		},
		{
			name: "Kafka Kernel Telemetry",
			context: testContext{
				serverPort:    kafkaPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        map[string]interface{}{},
			},
			testBody: func(t *testing.T, ctx *testContext, monitor *Monitor) {
				tests := []struct {
					name                string
					topicName           string
					expectedBucketIndex int
				}{
					{name: "Topic size is 1", topicName: strings.Repeat("a", 1), expectedBucketIndex: 0},
					{name: "Topic size is 10", topicName: strings.Repeat("a", 10), expectedBucketIndex: 0},
					{name: "Topic size is 20", topicName: strings.Repeat("a", 20), expectedBucketIndex: 1},
					{name: "Topic size is 30", topicName: strings.Repeat("a", 30), expectedBucketIndex: 2},
					{name: "Topic size is 40", topicName: strings.Repeat("a", 40), expectedBucketIndex: 3},
					{name: "Topic size is 10 again", topicName: strings.Repeat("a", 10), expectedBucketIndex: 0},
					{name: "Topic size is 50", topicName: strings.Repeat("a", 50), expectedBucketIndex: 4},
					{name: "Topic size is 60", topicName: strings.Repeat("a", 60), expectedBucketIndex: 5},
					{name: "Topic size is 70", topicName: strings.Repeat("a", 70), expectedBucketIndex: 6},
					{name: "Topic size is 79", topicName: strings.Repeat("a", 79), expectedBucketIndex: 7},
					{name: "Topic size is 80", topicName: strings.Repeat("a", 80), expectedBucketIndex: 7},
					{name: "Topic size is 81", topicName: strings.Repeat("a", 81), expectedBucketIndex: 8},
					{name: "Topic size is 90", topicName: strings.Repeat("a", 90), expectedBucketIndex: 8},
					{name: "Topic size is 100", topicName: strings.Repeat("a", 100), expectedBucketIndex: 9},
					{name: "Topic size is 120", topicName: strings.Repeat("a", 120), expectedBucketIndex: 9},
				}

				currentRawKernelTelemetry := &kafka.RawKernelTelemetry{}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						client, err := kafka.NewClient(kafka.Options{
							ServerAddress: ctx.targetAddress,
							DialFn:        dialFn,
							CustomOptions: []kgo.Opt{
								kgo.MaxVersions(version),
								kgo.ConsumeTopics(tt.topicName),
								kgo.ClientID("test-client"),
							},
						})
						require.NoError(t, err)
						ctx.clients = append(ctx.clients, client)
						require.NoError(t, client.CreateTopic(tt.topicName))

						record := &kgo.Record{Topic: tt.topicName, Value: []byte("Hello Kafka!")}
						ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
						defer cancel()
						require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")

						var telemetryMap *kafka.RawKernelTelemetry
						require.Eventually(t, func() bool {
							telemetryMap, err = kafka.GetKernelTelemetryMap(monitor.ebpfProgram.Manager.Manager)
							require.NoError(t, err)

							// Ensure that the other buckets remain unchanged before verifying the expected bucket.
							for idx := 0; idx < kafka.TopicNameBuckets; idx++ {
								if idx != tt.expectedBucketIndex {
									require.Equal(t, currentRawKernelTelemetry.Name_size_buckets[idx],
										telemetryMap.Name_size_buckets[idx],
										"Expected bucket (%d) to remain unchanged", idx)
								}
							}

							// Verify that the expected bucket contains the correct number of occurrences.
							expectedNumberOfOccurrences := fixCount(2) // (1 produce request + 1 fetch request)
							return uint64(expectedNumberOfOccurrences)+currentRawKernelTelemetry.Name_size_buckets[tt.expectedBucketIndex] == telemetryMap.Name_size_buckets[tt.expectedBucketIndex]
						}, time.Second*3, time.Millisecond*100)

						// Update the current raw kernel telemetry for the next iteration
						currentRawKernelTelemetry = telemetryMap
					})
				}
			},
		},
	}

	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, serverAddress, tls)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	cfg := getDefaultTestConfiguration(tls)
	monitor := newKafkaMonitor(t, cfg)
	if tls && cfg.EnableGoTLSSupport {
		utils.WaitForProgramsToBeTraced(t, "go-tls", proxyProcess.Process.Pid)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				for _, client := range tt.context.clients {
					client.Client.Close()
				}
				kafka.CleanKafkaMaps(t)
			})

			tt.testBody(t, &tt.context, monitor)
		})
	}
}

func generateFetchRequest(apiVersion int, topic string) kmsg.FetchRequest {
	req := kmsg.NewFetchRequest()
	req.SetVersion(int16(apiVersion))
	reqTopic := kmsg.NewFetchRequestTopic()
	reqTopic.Topic = topic
	partition := kmsg.NewFetchRequestTopicPartition()
	partition.PartitionMaxBytes = 1024
	reqTopic.Partitions = append(reqTopic.Partitions, partition)
	req.Topics = append(req.Topics, reqTopic)
	return req
}

func makeRecordWithVal(val []byte) kmsg.Record {
	var tmp []byte
	record := kmsg.NewRecord()
	record.Value = val
	tmp = record.AppendTo(make([]byte, 0))
	// 1 is the length of varint encoded 0
	record.Length = int32(len(tmp) - 1)
	return record
}

func makeRecord() kmsg.Record {
	return makeRecordWithVal([]byte("Hello Kafka!"))
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

func makeFetchResponse(apiVersion int, topics ...kmsg.FetchResponseTopic) kmsg.FetchResponse {
	resp := kmsg.NewFetchResponse()
	resp.SetVersion(int16(apiVersion))
	resp.ThrottleMillis = 999999999
	resp.SessionID = 0x11223344
	resp.Topics = append(resp.Topics, topics...)
	return resp
}

func appendUint32(dst []byte, u uint32) []byte {
	return append(dst, byte(u>>24), byte(u>>16), byte(u>>8), byte(u))
}

// kmsg doesn't have a ResponseFormatter so we need to add the length
// and the correlation Id ourselves.
func appendResponse(dst []byte, response kmsg.FetchResponse, correlationID uint32) []byte {
	var data []byte
	data = response.AppendTo(data)

	// +4 for correlationId
	length := uint32(len(data)) + 4
	if response.IsFlexible() {
		// Tagged Values
		length++
	}

	dst = appendUint32(dst, length)
	dst = appendUint32(dst, correlationID)

	if response.IsFlexible() {
		var numTags uint8
		dst = append(dst, numTags)
	}

	dst = append(dst, data...)

	return dst
}

type Message struct {
	request  []byte
	response []byte
}

func appendMessages(messages []Message, correlationID int, req kmsg.FetchRequest, resp kmsg.FetchResponse) []Message {
	formatter := kmsg.NewRequestFormatter(kmsg.FormatterClientID("kgo"))
	data := formatter.AppendRequest(make([]byte, 0), &req, int32(correlationID))
	respData := appendResponse(make([]byte, 0), resp, uint32(correlationID))

	return append(messages,
		Message{request: data},
		Message{response: respData},
	)
}

// CannedClientServer allows running a TCP server/client pair, optionally
// using TLS, which allows sending a list of canned messages comprising
// of requests and responses between the client and the server. This
// allows fine-graned control about where the boundaries between data
// chunks go, enabling us to verify the parsing continuation handling.
type CannedClientServer struct {
	control  chan []Message
	done     chan bool
	unixPath string
	address  string
	tls      bool
	t        *testing.T
}

func newCannedClientServer(t *testing.T, tls bool) *CannedClientServer {
	return &CannedClientServer{
		control:  make(chan []Message, 100),
		done:     make(chan bool, 1),
		unixPath: "/tmp/transparent.sock",
		// Use a different port than 9092 since the docker support code doesn't wait
		// for the container with the real Kafka server used in previous tests to terminate,
		// which leads to races. The disadvantage of not using 9092 is that you may
		// have to explicitly pick the protocol in Wireshark when debugging with a packet
		// trace.
		address: "127.0.0.1:8082",
		tls:     tls,
		t:       t,
	}
}

func (can *CannedClientServer) runServer() {
	var listener net.Listener
	var err error
	var f *os.File
	if can.tls {
		curDir, _ := testutil.CurDir()
		crtPath := filepath.Join(curDir, "../protocols/http/testutil/testdata/cert.pem.0")
		keyPath := filepath.Join(curDir, "../protocols/http/testutil/testdata/server.key")
		cer, err2 := tls.LoadX509KeyPair(crtPath, keyPath)
		require.NoError(can.t, err2)

		config := &tls.Config{Certificates: []tls.Certificate{cer}}

		// Only for decoding TLS with Wireshark. Disabled by default since it can result
		// in strange errors later if permissions/ownership are wrong on this file.
		// f, err := os.OpenFile("/tmp/ssl.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		// if err != nil {
		// 	config.KeyLogWriter = f
		// }

		listener, err = tls.Listen("tcp", can.address, config)
	} else {
		listener, err = net.Listen("tcp", can.address)
	}
	require.NoError(can.t, err)

	can.t.Cleanup(func() {
		close(can.control)
		<-can.done
	})

	go func() {
		defer func() {
			listener.Close()
			f.Close()
			can.done <- true
		}()

		conn, err := listener.Accept()
		require.NoError(can.t, err)
		conn.Close()

		// Delay close of connections to work around the known issue of races
		// between `tcp_close()` and the uprobes.  On the client side, the
		// connection is only closed after waiting for the stats.
		var prevconn net.Conn

		for msgs := range can.control {
			if prevconn != nil {
				prevconn.Close()
			}
			conn, err = listener.Accept()
			require.NoError(can.t, err)

			reader := bufio.NewReader(conn)
			for _, msg := range msgs {
				if len(msg.request) > 0 {
					_, err := io.ReadFull(reader, msg.request)
					require.NoError(can.t, err)
				}

				if len(msg.response) > 0 {
					conn.Write(msg.response)
				}
			}

			prevconn = conn
		}

		if prevconn != nil {
			prevconn.Close()
		}
	}()
}

func (can *CannedClientServer) runProxy() int {
	proxyProcess, cancel := proxy.NewExternalUnixControlProxyServer(can.t, can.unixPath, can.address, can.tls)
	can.t.Cleanup(cancel)
	require.NoError(can.t, proxy.WaitForConnectionReady(can.unixPath))

	return proxyProcess.Process.Pid
}

func (can *CannedClientServer) runClient(msgs []Message) {
	can.control <- msgs

	conn, err := net.Dial("unix", can.unixPath)
	require.NoError(can.t, err)
	can.t.Cleanup(func() { _ = conn.Close() })

	reader := bufio.NewReader(conn)
	for _, msg := range msgs {
		buf := make([]byte, 0)
		buf = binary.BigEndian.AppendUint64(buf, uint64(len(msg.request)))
		conn.Write(buf)

		if len(msg.request) > 0 {
			// Note that the net package sets TCP_NODELAY by default,
			// so this will send out each msg individually, which
			// is which we want to test split segment handling.
			conn.Write(msg.request)
		}

		buf = make([]byte, 0)
		buf = binary.BigEndian.AppendUint64(buf, uint64(len(msg.response)))
		conn.Write(buf)

		if len(msg.response) > 0 {
			_, err := io.ReadFull(reader, msg.response)
			require.NoError(can.t, err)
		}
	}
}

func testKafkaFetchRaw(t *testing.T, tls bool, apiVersion int) {
	defaultTopic := "test-topic"

	tests := []struct {
		name              string
		topic             string
		buildResponse     func(string) kmsg.FetchResponse
		buildMessages     func(kmsg.FetchRequest, kmsg.FetchResponse) []Message
		onlyTLS           bool
		numFetchedRecords int
	}{
		{
			name:  "basic",
			topic: defaultTopic,
			buildResponse: func(topic string) kmsg.FetchResponse {
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

				return makeFetchResponse(apiVersion, makeFetchResponseTopic(topic, partitions...))
			},
			numFetchedRecords: 5 * 4 * 3,
		},
		{
			name:  "large topic name",
			topic: strings.Repeat("a", 254) + "b",
			buildResponse: func(topic string) kmsg.FetchResponse {
				return makeFetchResponse(apiVersion, makeFetchResponseTopic(topic, makeFetchResponseTopicPartition(makeRecordBatch(makeRecord()))))
			},
			numFetchedRecords: 1,
		},
		{
			name:  "many partitions",
			topic: defaultTopic,
			buildResponse: func(topic string) kmsg.FetchResponse {
				// Use a minimal record size in order to pack partitions more
				// tightly and ensure that the program will have to parse more
				// partitions per segment (using many tail calls, etc).
				record := makeRecordWithVal([]byte(""))
				var records []kmsg.Record
				for i := 0; i < 1; i++ {
					records = append(records, record)
				}

				recordBatch := makeRecordBatch(records...)
				var batches []kmsg.RecordBatch
				for i := 0; i < 1; i++ {
					batches = append(batches, recordBatch)
				}

				partition := makeFetchResponseTopicPartition(batches...)
				var partitions []kmsg.FetchResponseTopicPartition
				for i := 0; i < 25; i++ {
					partitions = append(partitions, partition)
				}

				return makeFetchResponse(apiVersion, makeFetchResponseTopic(topic, partitions...))
			},
			numFetchedRecords: 1 * 1 * 25,
		},
		{
			name:  "many topics",
			topic: defaultTopic,
			buildResponse: func(topic string) kmsg.FetchResponse {
				// Use a minimal record size in order to pack topics more
				// tightly and ensure that the program will have to parse more
				// partitions per segment (using many tail calls, etc).
				record := makeRecordWithVal([]byte(""))
				var records []kmsg.Record
				for i := 0; i < 1; i++ {
					records = append(records, record)
				}

				recordBatch := makeRecordBatch(records...)
				var batches []kmsg.RecordBatch
				for i := 0; i < 1; i++ {
					batches = append(batches, recordBatch)
				}

				partition := makeFetchResponseTopicPartition(batches...)
				var partitions []kmsg.FetchResponseTopicPartition
				for i := 0; i < 1; i++ {
					partitions = append(partitions, partition)
				}

				var topics []kmsg.FetchResponseTopic
				topics = append(topics, makeFetchResponseTopic(topic, partitions...))
				// These topics will be ignored in the current implementation,
				// but we're adding them to ensure that we parse the number of
				// topics correctly.
				for i := 0; i < 128; i++ {
					topics = append(topics, makeFetchResponseTopic(fmt.Sprintf("empty-%d", i), partitions...))
				}

				return makeFetchResponse(apiVersion, topics...)
			},
			numFetchedRecords: 1,
		},
		{
			// franz-go reads the size first
			name:    "message size read first",
			onlyTLS: true,
			topic:   defaultTopic,
			buildResponse: func(topic string) kmsg.FetchResponse {
				record := makeRecord()
				partition := makeFetchResponseTopicPartition(makeRecordBatch(record))
				return makeFetchResponse(apiVersion, makeFetchResponseTopic(topic, partition))
			},
			buildMessages: func(req kmsg.FetchRequest, resp kmsg.FetchResponse) []Message {
				formatter := kmsg.NewRequestFormatter(kmsg.FormatterClientID("kgo"))
				var msgs []Message
				reqData := formatter.AppendRequest(make([]byte, 0), &req, int32(55))
				respData := appendResponse(make([]byte, 0), resp, uint32(55))

				msgs = append(msgs, Message{request: reqData})
				msgs = append(msgs, Message{response: respData[0:4]})
				msgs = append(msgs, Message{response: respData[4:]})
				return msgs
			},
			numFetchedRecords: 1,
		},
		{
			// librdkafka reads the message size and the correlation id first
			name:    "message size and correlation ID read first",
			onlyTLS: true,
			topic:   defaultTopic,
			buildResponse: func(topic string) kmsg.FetchResponse {
				record := makeRecord()
				partition := makeFetchResponseTopicPartition(makeRecordBatch(record))
				return makeFetchResponse(apiVersion, makeFetchResponseTopic(topic, partition))
			},
			buildMessages: func(req kmsg.FetchRequest, resp kmsg.FetchResponse) []Message {
				formatter := kmsg.NewRequestFormatter(kmsg.FormatterClientID("kgo"))
				var msgs []Message
				reqData := formatter.AppendRequest(make([]byte, 0), &req, int32(55))
				respData := appendResponse(make([]byte, 0), resp, uint32(55))

				msgs = append(msgs, Message{request: reqData})
				msgs = append(msgs, Message{response: respData[0:8]})
				msgs = append(msgs, Message{response: respData[8:]})
				return msgs
			},
			numFetchedRecords: 1,
		},
		{
			// kafka-go reads the message size and the correlation id separately
			name:    "message size first, then correlation ID",
			onlyTLS: true,
			topic:   defaultTopic,
			buildResponse: func(topic string) kmsg.FetchResponse {
				record := makeRecord()
				partition := makeFetchResponseTopicPartition(makeRecordBatch(record))
				return makeFetchResponse(apiVersion, makeFetchResponseTopic(topic, partition))
			},
			buildMessages: func(req kmsg.FetchRequest, resp kmsg.FetchResponse) []Message {
				formatter := kmsg.NewRequestFormatter(kmsg.FormatterClientID("kgo"))
				var msgs []Message
				reqData := formatter.AppendRequest(make([]byte, 0), &req, int32(55))
				respData := appendResponse(make([]byte, 0), resp, uint32(55))

				msgs = append(msgs, Message{request: reqData})
				msgs = append(msgs, Message{response: respData[0:4]})
				msgs = append(msgs, Message{response: respData[4:8]})
				msgs = append(msgs, Message{response: respData[8:]})
				return msgs
			},
			numFetchedRecords: 1,
		},
		{
			name:  "aborted transactions",
			topic: defaultTopic,
			buildResponse: func(topic string) kmsg.FetchResponse {
				record := makeRecord()
				partition := makeFetchResponseTopicPartition(makeRecordBatch(record, record))
				aborted := kmsg.NewFetchResponseTopicPartitionAbortedTransaction()

				for i := 0; i < 10; i++ {
					partition.AbortedTransactions = append(partition.AbortedTransactions, aborted)
				}

				return makeFetchResponse(apiVersion, makeFetchResponseTopic(topic, partition))
			},
			numFetchedRecords: 2,
		},
		{
			name:  "partial record batch",
			topic: defaultTopic,
			buildResponse: func(topic string) kmsg.FetchResponse {
				record := makeRecord()
				recordBatch := makeRecordBatch(record, record, record)
				partition := makeFetchResponseTopicPartition(recordBatch)

				// Partial record batch, aka "Truncated Content" in Wireshark.  See
				// comment near FetchResponseTopicPartition.RecordBatch in kmsg.
				tmp := recordBatch.AppendTo(make([]byte, 0))
				partition.RecordBatches = append(partition.RecordBatches, tmp[:len(tmp)-1]...)

				return makeFetchResponse(apiVersion, makeFetchResponseTopic(topic, partition))
			},
			numFetchedRecords: 3,
		},
	}

	can := newCannedClientServer(t, tls)
	can.runServer()
	proxyPid := can.runProxy()

	monitor := newKafkaMonitor(t, getDefaultTestConfiguration(tls))
	if tls {
		utils.WaitForProgramsToBeTraced(t, "go-tls", proxyPid)
	}

	for _, tt := range tests {
		if tt.onlyTLS && !tls {
			continue
		}

		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				kafka.CleanKafkaMaps(t)
			})
			req := generateFetchRequest(apiVersion, tt.topic)
			resp := tt.buildResponse(tt.topic)
			var msgs []Message

			if tt.buildMessages == nil {
				msgs = appendMessages(msgs, 99, req, resp)
			} else {
				msgs = tt.buildMessages(req, resp)
			}

			can.runClient(msgs)

			getAndValidateKafkaStats(t, monitor, 1, tt.topic, kafkaParsingValidation{
				expectedNumberOfFetchRequests: tt.numFetchedRecords,
				expectedAPIVersionFetch:       apiVersion,
				tlsEnabled:                    tls,
			})
		})

		// Test with buildMessages have custom splitters
		if tt.buildMessages != nil {
			continue
		}

		name := fmt.Sprintf("split/%s", tt.name)
		t.Run(name, func(t *testing.T) {
			if tls {
				t.Skip("TLS split tests disabled due to flakiness")
			}

			t.Cleanup(func() {
				kafka.CleanKafkaMaps(t)
			})
			req := generateFetchRequest(apiVersion, tt.topic)
			resp := tt.buildResponse(tt.topic)

			formatter := kmsg.NewRequestFormatter(kmsg.FormatterClientID("kgo"))

			var msgs []Message
			splitIdx := 0
			for splitIdx = 0; splitIdx < 1000; splitIdx++ {
				reqData := formatter.AppendRequest(make([]byte, 0), &req, int32(splitIdx))
				respData := appendResponse(make([]byte, 0), resp, uint32(splitIdx))

				// There is an assumption in the code that there are no splits
				// inside the header.
				minSegSize := 8

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

			can.runClient(msgs)
			getAndValidateKafkaStats(t, monitor, 1, tt.topic, kafkaParsingValidation{
				expectedNumberOfFetchRequests: tt.numFetchedRecords * splitIdx,
				expectedAPIVersionFetch:       apiVersion,
				tlsEnabled:                    tls,
			})
		})
	}
}

func (s *KafkaProtocolParsingSuite) TestKafkaFetchRaw() {
	t := s.T()
	versions := []int{4, 5, 7, 11, 12}

	t.Run("without TLS", func(t *testing.T) {
		for _, version := range versions {
			t.Run(fmt.Sprintf("api%d", version), func(t *testing.T) {
				testKafkaFetchRaw(t, false, version)
			})
		}
	})

	t.Run("with TLS", func(t *testing.T) {
		if !gotlsutils.GoTLSSupported(t, config.New()) {
			t.Skip("GoTLS not supported for this setup")
		}

		for _, version := range versions {
			t.Run(fmt.Sprintf("api%d", version), func(t *testing.T) {
				testKafkaFetchRaw(t, true, version)
			})
		}
	})
}

func TestKafkaInFlightMapCleaner(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	cfg := getDefaultTestConfiguration(false)
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

func getAndValidateKafkaStats(t *testing.T, monitor *Monitor, expectedStatsCount int, topicName string, validation kafkaParsingValidation) map[kafka.Key]*kafka.RequestStat {
	kafkaStats := make(map[kafka.Key]*kafka.RequestStat)
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
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
		assert.Equal(collect, expectedStatsCount, len(kafkaStats), "Did not find expected number of stats")
		if expectedStatsCount != 0 {
			validateProduceFetchCount(collect, kafkaStats, topicName, validation)
		}
	}, time.Second*5, time.Millisecond*100)
	return kafkaStats
}

func validateProduceFetchCount(t *assert.CollectT, kafkaStats map[kafka.Key]*kafka.RequestStat, topicName string, validation kafkaParsingValidation) {
	numberOfProduceRequests := 0
	numberOfFetchRequests := 0
	for kafkaKey, kafkaStat := range kafkaStats {
		hasTLSTag := kafkaStat.StaticTags&network.ConnTagGo != 0
		if hasTLSTag != validation.tlsEnabled {
			continue
		}
		assert.Equal(t, topicName[:min(len(topicName), 80)], kafkaKey.TopicName)
		switch kafkaKey.RequestAPIKey {
		case kafka.ProduceAPIKey:
			assert.Equal(t, uint16(validation.expectedAPIVersionProduce), kafkaKey.RequestVersion)
			numberOfProduceRequests += kafkaStat.Count
		case kafka.FetchAPIKey:
			assert.Equal(t, uint16(validation.expectedAPIVersionFetch), kafkaKey.RequestVersion)
			numberOfFetchRequests += kafkaStat.Count
		default:
			assert.FailNow(t, "Expecting only produce or fetch kafka requests")
		}
	}
	assert.Equal(t, validation.expectedNumberOfProduceRequests, numberOfProduceRequests,
		"Expected %d produce requests but got %d", validation.expectedNumberOfProduceRequests, numberOfProduceRequests)
	assert.Equal(t, validation.expectedNumberOfFetchRequests, numberOfFetchRequests,
		"Expected %d fetch requests but got %d", validation.expectedNumberOfFetchRequests, numberOfFetchRequests)
}

func getDefaultTestConfiguration(tls bool) *config.Config {
	cfg := config.New()
	cfg.EnableKafkaMonitoring = true
	cfg.MaxTrackedConnections = 1000
	if tls {
		cfg.EnableGoTLSSupport = true
		cfg.GoTLSExcludeSelf = true
	}
	return cfg
}

func newKafkaMonitor(t *testing.T, cfg *config.Config) *Monitor {
	monitor, err := NewMonitor(cfg, nil)
	skipIfNotSupported(t, err)
	require.NoError(t, err)
	t.Cleanup(func() {
		monitor.Stop()
	})
	t.Cleanup(utils.ResetDebugger)

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
