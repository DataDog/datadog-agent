// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf || (windows && npm)
// +build linux_bpf windows,npm

package tracer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	redis2 "github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kversion"
	"github.com/uptrace/bun"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/amqp"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	protocolsmongo "github.com/DataDog/datadog-agent/pkg/network/protocols/mongo"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/mysql"
	pgutils "github.com/DataDog/datadog-agent/pkg/network/protocols/postgres"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/redis"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
)

const (
	defaultTimeout = 30 * time.Second
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

// protocolClassificationAttributes holds all attributes a single protocol classification test should have.
type protocolClassificationAttributes struct {
	// The name of the test.
	name string
	// Specific test context, allows to share states among different phases of the test.
	context testContext
	// Allows to decide on runtime if we should skip the test or not.
	skipCallback func(t *testing.T, ctx testContext)
	// Allows to do any preparation without traffic being captured by the tracer.
	preTracerSetup func(t *testing.T, ctx testContext)
	// All traffic here will be captured by the tracer.
	postTracerSetup func(t *testing.T, ctx testContext)
	// A validation method ensure the test succeeded.
	validation func(t *testing.T, ctx testContext, tr *Tracer)
	// Cleaning test resources if needed.
	teardown func(t *testing.T, ctx testContext)
}

func validateProtocolConnection(expectedProtocol network.ProtocolType) func(t *testing.T, ctx testContext, tr *Tracer) {
	return func(t *testing.T, ctx testContext, tr *Tracer) {
		waitForConnectionsWithProtocol(t, tr, ctx.targetAddress, ctx.serverAddress, expectedProtocol)
	}
}

// skipIfNotLinux skips the test if we are not on a linux machine
func skipIfNotLinux(t *testing.T, _ testContext) {
	if runtime.GOOS != "linux" {
		t.Skip("test is supported on linux machine only")
	}
}

// skipIfUsingNAT skips the test if we have a NAT rules applied.
func skipIfUsingNAT(t *testing.T, ctx testContext) {
	if ctx.targetAddress != ctx.serverAddress {
		t.Skip("test is not supported when NAT is applied")
	}
}

// composeSkips skips if one of the given filters is matched.
func composeSkips(skippers ...func(t *testing.T, ctx testContext)) func(t *testing.T, ctx testContext) {
	return func(t *testing.T, ctx testContext) {
		for _, skipFunction := range skippers {
			skipFunction(t, ctx)
		}
	}
}

const (
	mysqlPort    = "3306"
	postgresPort = "5432"
	mongoPort    = "27017"
	redisPort    = "6379"
	amqpPort     = "5672"
	httpPort     = "8080"
	tcpPort      = "9999"
	http2Port    = "9090"
	kafkaPort    = "9092"

	produceAPIKey = 0
	fetchAPIKey   = 1

	produceMaxSupportedVersion = 8
	produceMaxVersion          = 9
	produceMinSupportedVersion = 1
	produceMinVersion          = 0

	fetchMaxSupportedVersion = 11
	fetchMaxVersion          = 13
	fetchMinSupportedVersion = 0
	fetchMinVersion          = 0
)

func testProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string)
	}{
		{
			name:     "kafka",
			testFunc: testKafkaProtocolClassification,
		},
		{
			name:     "mysql",
			testFunc: testMySQLProtocolClassification,
		},
		{
			name:     "postgres",
			testFunc: testPostgresProtocolClassification,
		},
		{
			name:     "mongo",
			testFunc: testMongoProtocolClassification,
		},
		{
			name:     "redis",
			testFunc: testRedisProtocolClassification,
		},
		{
			name:     "amqp",
			testFunc: testAMQPProtocolClassification,
		},
		{
			name:     "http",
			testFunc: testHTTPProtocolClassification,
		},
		{
			name:     "http2",
			testFunc: testHTTP2ProtocolClassification,
		},
		{
			name:     "edge cases",
			testFunc: testEdgeCasesProtocolClassification,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t, tr, clientHost, targetHost, serverHost)
		})
	}
}

func testKafkaProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	const topicName = "franz-kafka"
	testIndex := 0
	// Kafka does not allow us to delete topic, but to mark them for deletion, so we have to generate a unique topic
	// per a test.
	getTopicName := func() string {
		testIndex++
		return fmt.Sprintf("%s-%d", topicName, testIndex)
	}

	skipFunc := composeSkips(skipIfNotLinux, skipIfUsingNAT)
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

	kafkaTeardown := func(t *testing.T, ctx testContext) {
		if _, ok := ctx.extras["client"]; !ok {
			return
		}
		client := ctx.extras["client"].(*kafka.Client)
		defer client.Client.Close()
		for k, value := range ctx.extras {
			if strings.HasPrefix(k, "topic_name") {
				// We're in the teardown phase, and deleting the topic name is a best effort operation. Therefore, we can ignore any errors that may occur.
				_ = client.DeleteTopic(value.(string))
			}
		}
	}

	serverAddress := net.JoinHostPort(serverHost, kafkaPort)
	targetAddress := net.JoinHostPort(targetHost, kafkaPort)
	require.True(t, kafka.RunServer(t, serverHost, kafkaPort))

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
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(kversion.V0_10_1())},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			validation: validateProtocolConnection(network.ProtocolUnknown),
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
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(kversion.V0_10_1())},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*kafka.Client)
				require.NoError(t, client.CreateTopic(ctx.extras["topic_name"].(string)))
			},
			validation: validateProtocolConnection(network.ProtocolUnknown),
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
					Dialer:        defaultDialer,
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
			validation: validateProtocolConnection(network.ProtocolKafka),
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
					Dialer:        defaultDialer,
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
			validation: validateProtocolConnection(network.ProtocolKafka),
			teardown:   kafkaTeardown,
		},
	}

	// Adding produce tests in different versions
	for i := int16(produceMinVersion); i <= produceMaxVersion; i++ {
		version := kversion.V3_4_0()
		expectedProtocol := network.ProtocolKafka
		if i < produceMinSupportedVersion || i > produceMaxSupportedVersion {
			expectedProtocol = network.ProtocolUnknown
		}
		version.SetMaxKeyVersion(produceAPIKey, i)
		tests = append(tests, protocolClassificationAttributes{
			name: fmt.Sprintf("produce - version %d", i),
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
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(version)},
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
			validation: validateProtocolConnection(expectedProtocol),
			teardown:   kafkaTeardown,
		})
	}
	// Adding fetch tests in different versions
	for i := int16(fetchMinVersion); i < fetchMaxVersion; i++ {
		expectedProtocol := network.ProtocolKafka
		if i < fetchMinSupportedVersion || i > fetchMaxSupportedVersion {
			expectedProtocol = network.ProtocolUnknown
		}
		version := kversion.V3_4_0()
		version.SetMaxKeyVersion(fetchAPIKey, i)
		tests = append(tests, protocolClassificationAttributes{
			name: fmt.Sprintf("fetch - sanity version %d", i),
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
					Dialer:        defaultDialer,
					CustomOptions: []kgo.Opt{kgo.MaxVersions(version), kgo.ConsumeTopics(ctx.extras["topic_name"].(string))},
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				require.NoError(t, client.CreateTopic(ctx.extras["topic_name"].(string)))
				record := &kgo.Record{Topic: ctx.extras["topic_name"].(string), Value: []byte("Hello Kafka!")}
				ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				require.NoError(t, client.Client.ProduceSync(ctxTimeout, record).FirstErr(), "record had a produce error while synchronously producing")
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*kafka.Client)
				fetches := client.Client.PollFetches(context.Background())
				require.Empty(t, fetches.Errors())
				records := fetches.Records()
				require.Len(t, records, 1)
				require.Equal(t, ctx.extras["topic_name"].(string), records[0].Topic)
			},
			validation: validateProtocolConnection(expectedProtocol),
			teardown:   kafkaTeardown,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testMySQLProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	skipFunc := composeSkips(skipIfNotLinux, skipIfUsingNAT)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    mysqlPort,
		targetAddress: targetHost,
	})

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	mysqlTeardown := func(t *testing.T, ctx testContext) {
		client := ctx.extras["conn"].(*mysql.Client)
		defer client.DB.Close()
		client.DropDB()
	}

	serverAddress := net.JoinHostPort(serverHost, mysqlPort)
	targetAddress := net.JoinHostPort(targetHost, mysqlPort)
	require.True(t, mysql.RunServer(t, serverHost, mysqlPort))

	tests := []protocolClassificationAttributes{
		{
			name: "connect",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
			},
			validation: validateProtocolConnection(network.ProtocolMySQL),
			teardown:   mysqlTeardown,
		},
		{
			name: "create db",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.CreateDB())
			},
			validation: validateProtocolConnection(network.ProtocolMySQL),
			teardown:   mysqlTeardown,
		},
		{
			name: "create table",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, c.CreateDB())
				ctx.extras["conn"] = c
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.CreateTable())
			},
			validation: validateProtocolConnection(network.ProtocolMySQL),
			teardown:   mysqlTeardown,
		},
		{
			name: "insert",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				ctx.extras["conn"] = c
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.InsertIntoTable("Bratislava", 432000))
			},
			validation: validateProtocolConnection(network.ProtocolMySQL),
			teardown:   mysqlTeardown,
		},
		{
			name: "delete",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["conn"] = c
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				require.NoError(t, c.InsertIntoTable("Bratislava", 432000))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.DeleteFromTable("Bratislava"))
			},
			validation: validateProtocolConnection(network.ProtocolMySQL),
			teardown:   mysqlTeardown,
		},
		{
			name: "select",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				require.NoError(t, c.InsertIntoTable("Bratislava", 432000))
				ctx.extras["conn"] = c
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				population, err := c.SelectFromTable("Bratislava")
				require.NoError(t, err)
				require.Equal(t, 432000, population)
			},
			validation: validateProtocolConnection(network.ProtocolMySQL),
			teardown:   mysqlTeardown,
		},
		{
			name: "update",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				require.NoError(t, c.InsertIntoTable("Bratislava", 432000))
				ctx.extras["conn"] = c
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.UpdateTable("Bratislava", "Bratislava2", 10))
			},
			validation: validateProtocolConnection(network.ProtocolMySQL),
			teardown:   mysqlTeardown,
		},
		{
			name: "drop table",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				ctx.extras["conn"] = c
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.DropTable())
			},
			validation: validateProtocolConnection(network.ProtocolMySQL),
			teardown:   mysqlTeardown,
		},
		{
			name: "alter",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				ctx.extras["conn"] = c
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.AlterTable())
			},
			validation: validateProtocolConnection(network.ProtocolMySQL),
			teardown:   mysqlTeardown,
		},
		{
			// Test that we classify long queries that would be
			// splitted between multiple packets correctly
			name: "long query",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				ctx.extras["conn"] = c
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.InsertIntoTable(strings.Repeat("#", 16384), 10))
			},
			validation: validateProtocolConnection(network.ProtocolMySQL),
			teardown:   mysqlTeardown,
		},
		{
			// Test that we classify long queries that would be
			// splitted between multiple packets correctly
			name: "long response",
			context: testContext{
				serverPort:    mysqlPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := mysql.NewClient(mysql.Options{
					ServerAddress: ctx.targetAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, c.CreateDB())
				require.NoError(t, c.CreateTable())
				name := strings.Repeat("#", 1024)
				for i := int64(1); i <= 40; i++ {
					require.NoError(t, c.InsertIntoTable(name+"i", 10))
				}
				ctx.extras["conn"] = c
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c := ctx.extras["conn"].(*mysql.Client)
				require.NoError(t, c.SelectAllFromTable())
			},
			validation: validateProtocolConnection(network.ProtocolMySQL),
			teardown:   mysqlTeardown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testPostgresProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	skipFunc := composeSkips(skipIfNotLinux, skipIfUsingNAT)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    postgresPort,
		targetAddress: targetHost,
	})

	if clientHost != "127.0.0.1" {
		t.Skip("postgres tests are not supported DNat")
	}

	postgresTeardown := func(t *testing.T, ctx testContext) {
		db := ctx.extras["db"].(*bun.DB)
		defer db.Close()
		taskCtx := ctx.extras["ctx"].(context.Context)
		_, _ = db.NewDropTable().Model((*pgutils.DummyTable)(nil)).Exec(taskCtx)
	}

	// Setting one instance of postgres server for all tests.
	serverAddress := net.JoinHostPort(serverHost, postgresPort)
	targetAddress := net.JoinHostPort(targetHost, postgresPort)
	require.True(t, pgutils.RunServer(t, serverHost, postgresPort))

	tests := []protocolClassificationAttributes{
		{
			name: "postgres - connect",
			context: testContext{
				serverPort:    postgresPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pg := pgutils.GetPGHandle(t, ctx.serverAddress)
				conn, err := pg.Conn(context.Background())
				require.NoError(t, err)
				defer conn.Close()
			},
			validation: validateProtocolConnection(network.ProtocolPostgres),
			teardown:   postgresTeardown,
		},
		{
			name: "postgres - insert",
			context: testContext{
				serverPort:    postgresPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				pgutils.RunCreateQuery(t, ctx.extras)
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.RunInsertQuery(t, 1, ctx.extras)
			},
			validation: validateProtocolConnection(network.ProtocolPostgres),
			teardown:   postgresTeardown,
		},
		{
			name: "postgres - delete",
			context: testContext{
				serverPort:    postgresPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				pgutils.RunCreateQuery(t, ctx.extras)
				pgutils.RunInsertQuery(t, 1, ctx.extras)
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.RunDeleteQuery(t, ctx.extras)
			},
			validation: validateProtocolConnection(network.ProtocolPostgres),
			teardown:   postgresTeardown,
		},
		{
			name: "postgres - select",
			context: testContext{
				serverPort:    postgresPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				pgutils.RunCreateQuery(t, ctx.extras)
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.RunSelectQuery(t, ctx.extras)
			},
			validation: validateProtocolConnection(network.ProtocolPostgres),
			teardown:   postgresTeardown,
		},
		{
			name: "postgres - update",
			context: testContext{
				serverPort:    postgresPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				pgutils.RunCreateQuery(t, ctx.extras)
				pgutils.RunInsertQuery(t, 1, ctx.extras)
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.RunUpdateQuery(t, ctx.extras)
			},
			validation: validateProtocolConnection(network.ProtocolPostgres),
			teardown:   postgresTeardown,
		},
		{
			name: "postgres - drop",
			context: testContext{
				serverPort:    postgresPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				pgutils.RunCreateQuery(t, ctx.extras)
				pgutils.RunInsertQuery(t, 1, ctx.extras)
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.RunDropQuery(t, ctx.extras)
			},
			validation: validateProtocolConnection(network.ProtocolPostgres),
			teardown:   postgresTeardown,
		},
		{
			name: "postgres - alter",
			context: testContext{
				serverPort:    postgresPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				pgutils.RunCreateQuery(t, ctx.extras)
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.RunAlterQuery(t, ctx.extras)
			},
			validation: validateProtocolConnection(network.ProtocolPostgres),
			teardown:   postgresTeardown,
		},
		{
			// Test that we classify long queries that would be
			// splitted between multiple packets correctly
			name: "postgres - long query",
			context: testContext{
				serverPort:    postgresPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				pgutils.RunCreateQuery(t, ctx.extras)
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				db := ctx.extras["db"].(*bun.DB)
				taskCtx := ctx.extras["ctx"].(context.Context)

				// This will fail but it should make a query and be classified
				_, _ = db.NewInsert().Model(&pgutils.DummyTable{Foo: strings.Repeat("#", 16384)}).Exec(taskCtx)
			},
			validation: validateProtocolConnection(network.ProtocolPostgres),
			teardown:   postgresTeardown,
		},
		{
			// Test that we classify long queries that would be
			// splitted between multiple packets correctly
			name: "postgres - long response",
			context: testContext{
				serverPort:    postgresPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				pgutils.RunCreateQuery(t, ctx.extras)
				for i := int64(1); i < 200; i++ {
					pgutils.RunInsertQuery(t, i, ctx.extras)
				}
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				pgutils.RunSelectQuery(t, ctx.extras)
			},
			validation: validateProtocolConnection(network.ProtocolPostgres),
			teardown:   postgresTeardown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testMongoProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	skipFunc := composeSkips(skipIfNotLinux, skipIfUsingNAT)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    mongoPort,
		targetAddress: targetHost,
	})

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	mongoTeardown := func(t *testing.T, ctx testContext) {
		client := ctx.extras["client"].(*protocolsmongo.Client)
		require.NoError(t, client.DeleteDatabases())
		defer client.Stop()
	}

	// Setting one instance of mongo server for all tests.
	serverAddress := net.JoinHostPort(serverHost, mongoPort)
	targetAddress := net.JoinHostPort(targetHost, mongoPort)
	require.True(t, protocolsmongo.RunServer(t, serverHost, mongoPort))

	tests := []protocolClassificationAttributes{
		{
			name: "classify by connect",
			context: testContext{
				serverPort:    mongoPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := protocolsmongo.NewClient(protocolsmongo.Options{
					ServerAddress: ctx.targetAddress,
					ClientDialer:  defaultDialer,
				})
				require.NoError(t, err)
				client.Stop()
			},
			validation: validateProtocolConnection(network.ProtocolMongo),
		},
		{
			name: "classify by collection creation",
			context: testContext{
				serverPort:    mongoPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := protocolsmongo.NewClient(protocolsmongo.Options{
					ServerAddress: ctx.targetAddress,
					ClientDialer:  defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*protocolsmongo.Client)
				db := client.C.Database("test")
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				require.NoError(t, db.CreateCollection(timedContext, "collection"))
			},
			validation: validateProtocolConnection(network.ProtocolMongo),
			teardown:   mongoTeardown,
		},
		{
			name: "classify by insertion",
			context: testContext{
				serverPort:    mongoPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := protocolsmongo.NewClient(protocolsmongo.Options{
					ServerAddress: ctx.targetAddress,
					ClientDialer:  defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				db := client.C.Database("test")
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				require.NoError(t, db.CreateCollection(timedContext, "collection"))
				ctx.extras["collection"] = db.Collection("collection")
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				collection := ctx.extras["collection"].(*mongo.Collection)
				input := map[string]string{"test": "test"}
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				_, err := collection.InsertOne(timedContext, input)
				require.NoError(t, err)
			},
			validation: validateProtocolConnection(network.ProtocolMongo),
			teardown:   mongoTeardown,
		},
		{
			name: "classify by find",
			context: testContext{
				serverPort:    mongoPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := protocolsmongo.NewClient(protocolsmongo.Options{
					ServerAddress: ctx.targetAddress,
					ClientDialer:  defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
				db := client.C.Database("test")
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				require.NoError(t, db.CreateCollection(timedContext, "collection"))
				cancel()
				collection := db.Collection("collection")
				ctx.extras["input"] = map[string]string{"test": "test"}
				timedContext, cancel = context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				_, err = collection.InsertOne(timedContext, ctx.extras["input"])
				require.NoError(t, err)

				ctx.extras["collection"] = db.Collection("collection")
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				collection := ctx.extras["collection"].(*mongo.Collection)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				res := collection.FindOne(timedContext, bson.M{"test": "test"})
				require.NoError(t, res.Err())
				var output map[string]string
				require.NoError(t, res.Decode(&output))
				delete(output, "_id")
				require.EqualValues(t, output, ctx.extras["input"])
			},
			validation: validateProtocolConnection(network.ProtocolMongo),
			teardown:   mongoTeardown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testRedisProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	skipFunc := composeSkips(skipIfNotLinux, skipIfUsingNAT)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    redisPort,
		targetAddress: targetHost,
	})

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	redisTeardown := func(t *testing.T, ctx testContext) {
		redis.NewClient(ctx.serverAddress, defaultDialer)
		client := ctx.extras["client"].(*redis2.Client)
		timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
		require.NoError(t, client.FlushDB(timedContext).Err())
	}

	// Setting one instance of redis server for all tests.
	serverAddress := net.JoinHostPort(serverHost, redisPort)
	targetAddress := net.JoinHostPort(targetHost, redisPort)
	require.True(t, redis.RunServer(t, serverHost, redisPort))

	tests := []protocolClassificationAttributes{
		{
			name: "set",
			context: testContext{
				serverPort:    redisPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client := redis.NewClient(ctx.targetAddress, defaultDialer)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				client.Ping(timedContext)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*redis2.Client)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				client.Set(timedContext, "key", "value", time.Minute)
			},
			teardown:   redisTeardown,
			validation: validateProtocolConnection(network.ProtocolRedis),
		},
		{
			name: "get",
			context: testContext{
				serverPort:    redisPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client := redis.NewClient(ctx.targetAddress, defaultDialer)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				client.Set(timedContext, "key", "value", time.Minute)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*redis2.Client)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				res := client.Get(timedContext, "key")
				val, err := res.Result()
				require.NoError(t, err)
				require.Equal(t, "value", val)
			},
			teardown:   redisTeardown,
			validation: validateProtocolConnection(network.ProtocolRedis),
		},
		{
			name: "get unknown key",
			context: testContext{
				serverPort:    redisPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client := redis.NewClient(ctx.targetAddress, defaultDialer)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				client.Ping(timedContext)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*redis2.Client)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				res := client.Get(timedContext, "unknown")
				require.Error(t, res.Err())
			},
			teardown:   redisTeardown,
			validation: validateProtocolConnection(network.ProtocolRedis),
		},
		{
			name: "err response",
			context: testContext{
				serverPort:    redisPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				conn, err := defaultDialer.DialContext(timedContext, "tcp", ctx.targetAddress)
				require.NoError(t, err)
				_, err = conn.Write([]byte("+dummy\r\n"))
				require.NoError(t, err)
			},
			validation: validateProtocolConnection(network.ProtocolRedis),
		},
		{
			name: "client id",
			context: testContext{
				serverPort:    redisPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client := redis.NewClient(ctx.targetAddress, defaultDialer)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				client.Ping(timedContext)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*redis2.Client)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				res := client.ClientID(timedContext)
				require.NoError(t, res.Err())
			},
			teardown:   redisTeardown,
			validation: validateProtocolConnection(network.ProtocolRedis),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testAMQPProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	skipFunc := composeSkips(skipIfNotLinux, skipIfUsingNAT)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    amqpPort,
		targetAddress: targetHost,
	})

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	amqpTeardown := func(t *testing.T, ctx testContext) {
		client := ctx.extras["client"].(*amqp.Client)
		defer client.Terminate()

		require.NoError(t, client.DeleteQueues())
	}

	// Setting one instance of amqp server for all tests.
	serverAddress := net.JoinHostPort(serverHost, amqpPort)
	targetAddress := net.JoinHostPort(targetHost, amqpPort)
	require.True(t, amqp.RunServer(t, serverHost, amqpPort))

	tests := []protocolClassificationAttributes{
		{
			name: "connect",
			context: testContext{
				serverPort:    amqpPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(amqp.Options{
					ServerAddress: ctx.serverAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(network.ProtocolAMQP),
		},
		{
			name: "declare channel",
			context: testContext{
				serverPort:    amqpPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(amqp.Options{
					ServerAddress: ctx.serverAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*amqp.Client)
				require.NoError(t, client.DeclareQueue("test", client.PublishChannel))
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(network.ProtocolUnknown),
		},
		{
			name: "publish",
			context: testContext{
				serverPort:    amqpPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(amqp.Options{
					ServerAddress: ctx.serverAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, client.DeclareQueue("test", client.PublishChannel))
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*amqp.Client)
				require.NoError(t, client.Publish("test", "my msg"))
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(network.ProtocolAMQP),
		},
		{
			name: "consume",
			context: testContext{
				serverPort:    amqpPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(amqp.Options{
					ServerAddress: ctx.serverAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, client.DeclareQueue("test", client.PublishChannel))
				require.NoError(t, client.DeclareQueue("test", client.ConsumeChannel))
				require.NoError(t, client.Publish("test", "my msg"))
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*amqp.Client)
				res, err := client.Consume("test", 1)
				require.NoError(t, err)
				require.Equal(t, []string{"my msg"}, res)
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(network.ProtocolAMQP),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testHTTPProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	serverAddress := net.JoinHostPort(serverHost, httpPort)
	targetAddress := net.JoinHostPort(targetHost, httpPort)
	tests := []protocolClassificationAttributes{
		{
			name: "tcp client with sending HTTP request",
			context: testContext{
				serverPort:    httpPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				ln, err := net.Listen("tcp", ctx.serverAddress)
				require.NoError(t, err)

				srv := &nethttp.Server{
					Addr: ln.Addr().String(),
					Handler: nethttp.HandlerFunc(func(w nethttp.ResponseWriter, req *nethttp.Request) {
						io.Copy(io.Discard, req.Body)
						w.WriteHeader(200)
					}),
					ReadTimeout:  time.Second,
					WriteTimeout: time.Second,
				}
				srv.SetKeepAlivesEnabled(false)
				go func() {
					_ = srv.Serve(ln)
				}()

				ctx.extras["server"] = srv
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := nethttp.Client{
					Transport: &nethttp.Transport{
						DialContext: defaultDialer.DialContext,
					},
				}
				resp, err := client.Get("http://" + ctx.targetAddress + "/test")
				require.NoError(t, err)
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			},
			teardown: func(t *testing.T, ctx testContext) {
				srv := ctx.extras["server"].(*nethttp.Server)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				_ = srv.Shutdown(timedContext)
			},
			validation: validateProtocolConnection(network.ProtocolHTTP),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testHTTP2ProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP:   net.ParseIP(clientHost),
			Port: 0,
		},
	}

	serverAddress := net.JoinHostPort(serverHost, http2Port)
	targetAddress := net.JoinHostPort(targetHost, http2Port)

	server, err := grpc.NewServer(serverAddress)
	require.NoError(t, err)
	server.Run()
	t.Cleanup(server.Stop)

	tests := []protocolClassificationAttributes{
		{
			name: "http2 traffic using gRPC - unary call",
			context: testContext{
				serverPort:    http2Port,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := grpc.NewClient(ctx.targetAddress, grpc.Options{
					CustomDialer: defaultDialer,
				})
				require.NoError(t, err)
				defer c.Close()
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				require.NoError(t, c.HandleUnary(timedContext, "test"))
			},
			validation: validateProtocolConnection(network.ProtocolHTTP2),
		},
		{
			name: "http2 traffic using gRPC - stream call",
			context: testContext{
				serverPort:    http2Port,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := grpc.NewClient(ctx.targetAddress, grpc.Options{
					CustomDialer: defaultDialer,
				})
				require.NoError(t, err)
				defer c.Close()
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				require.NoError(t, c.HandleStream(timedContext, 5))
			},
			validation: validateProtocolConnection(network.ProtocolHTTP2),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testEdgeCasesProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	teardown := func(t *testing.T, ctx testContext) {
		server, ok := ctx.extras["server"].(*TCPServer)
		if ok {
			server.Shutdown()
			// Giving time for the port to be free again.
			time.Sleep(time.Second)
		}
	}

	tests := []protocolClassificationAttributes{
		{
			name: "tcp client without sending data",
			context: testContext{
				serverPort:    tcpPort,
				serverAddress: net.JoinHostPort(serverHost, tcpPort),
				targetAddress: net.JoinHostPort(targetHost, tcpPort),
				extras:        map[string]interface{}{},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				server := NewTCPServerOnAddress(ctx.serverAddress, func(c net.Conn) {
					c.Close()
				})
				ctx.extras["server"] = server
				require.NoError(t, server.Run())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				timedContext, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				c, err := defaultDialer.DialContext(timedContext, "tcp", ctx.targetAddress)
				require.NoError(t, err)
				defer c.Close()
			},
			teardown:   teardown,
			validation: validateProtocolConnection(network.ProtocolUnknown),
		},
		{
			name: "tcp client with sending random data",
			context: testContext{
				serverPort:    tcpPort,
				serverAddress: net.JoinHostPort(serverHost, tcpPort),
				targetAddress: net.JoinHostPort(targetHost, tcpPort),
				extras:        map[string]interface{}{},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				server := NewTCPServerOnAddress(ctx.serverAddress, func(c net.Conn) {
					defer c.Close()
					r := bufio.NewReader(c)
					input, err := r.ReadBytes(byte('\n'))
					if err == nil {
						c.Write(input)
					}
				})
				ctx.extras["server"] = server
				require.NoError(t, server.Run())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				timedContext, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				c, err := defaultDialer.DialContext(timedContext, "tcp", ctx.targetAddress)
				require.NoError(t, err)
				defer c.Close()
				c.Write([]byte("hello\n"))
				io.ReadAll(c)
			},
			teardown:   teardown,
			validation: validateProtocolConnection(network.ProtocolUnknown),
		},
		{
			// A case where we see multiple protocols on the same socket. In that case, we expect to classify the connection
			// with the first protocol we've found.
			name: "mixed protocols",
			context: testContext{
				serverPort:    tcpPort,
				serverAddress: net.JoinHostPort(serverHost, tcpPort),
				targetAddress: net.JoinHostPort(targetHost, tcpPort),
				extras:        map[string]interface{}{},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				server := NewTCPServerOnAddress(ctx.serverAddress, func(c net.Conn) {
					defer c.Close()

					r := bufio.NewReader(c)
					input, err := r.ReadBytes(byte('\n'))
					if err == nil {
						c.Write(input)
					}
				})
				ctx.extras["server"] = server
				require.NoError(t, server.Run())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				timedContext, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				c, err := defaultDialer.DialContext(timedContext, "tcp", ctx.targetAddress)
				require.NoError(t, err)
				defer c.Close()
				c.Write([]byte("GET /200/foobar HTTP/1.1\n"))
				io.ReadAll(c)
				// http2 prefix.
				c.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
				io.ReadAll(c)
			},
			teardown:   teardown,
			validation: validateProtocolConnection(network.ProtocolHTTP),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func waitForConnectionsWithProtocol(t *testing.T, tr *Tracer, targetAddr, serverAddr string, expectedProtocol network.ProtocolType) {
	var incomingConns, outgoingConns []network.ConnectionStats

	foundIncomingWithProtocol := false
	foundOutgoingWithProtocol := false

	for start := time.Now(); time.Since(start) < 5*time.Second; {
		conns := getConnections(t, tr)
		newOutgoingConns := searchConnections(conns, func(cs network.ConnectionStats) bool {
			return cs.Type == network.TCP && fmt.Sprintf("%s:%d", cs.Dest, cs.DPort) == targetAddr
		})
		newIncomingConns := searchConnections(conns, func(cs network.ConnectionStats) bool {
			return cs.Type == network.TCP && fmt.Sprintf("%s:%d", cs.Source, cs.SPort) == serverAddr
		})

		outgoingConns = append(outgoingConns, newOutgoingConns...)
		incomingConns = append(incomingConns, newIncomingConns...)

		for _, conn := range newOutgoingConns {
			t.Logf("Found outgoing connection %v", conn)
			if conn.Protocol == expectedProtocol {
				foundOutgoingWithProtocol = true
				break
			}
		}
		for _, conn := range newIncomingConns {
			t.Logf("Found incoming connection %v", conn)
			if conn.Protocol == expectedProtocol {
				foundIncomingWithProtocol = true
				break
			}
		}

		if foundOutgoingWithProtocol && foundIncomingWithProtocol {
			return
		}

		time.Sleep(200 * time.Millisecond)
	}

	// If we didn't find both -> fail
	if foundOutgoingWithProtocol || foundIncomingWithProtocol {
		// We have found at least one.
		// Checking if the reason for not finding the other is flakiness of npm
		if !foundIncomingWithProtocol && len(incomingConns) == 0 {
			t.Log("npm didn't find the incoming connections, not failing the test")
			return
		}

		if !foundOutgoingWithProtocol && len(outgoingConns) == 0 {
			t.Log("npm didn't find the outgoing connections, not failing the test")
			return
		}

	}

	t.Errorf("couldn't find incoming and outgoing connections with protocol %d for "+
		"server address %s and target address %s.\nIncoming: %v\nOutgoing: %v\nfound incoming with protocol: "+
		"%v\nfound outgoing with protocol: %v", expectedProtocol, serverAddr, targetAddr, incomingConns, outgoingConns, foundIncomingWithProtocol, foundOutgoingWithProtocol)
}
