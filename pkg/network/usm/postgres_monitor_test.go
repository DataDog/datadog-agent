// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/uptrace/bun"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres"
)

const (
	postgresPort = "5432"
)

// postgresParsingTestAttributes holds all attributes a single postgres parsing test should have.
type postgresParsingTestAttributes struct {
	// The name of the test.
	name string
	// Specific test context, allows to share states among different phases of the test.
	context testContext
	// Allows to do any preparation without traffic being captured by the monitor.
	preMonitorSetup func(t *testing.T, ctx testContext)
	// All traffic here will be captured by the monitor.
	postMonitorSetup func(t *testing.T, ctx testContext)
	// A validation method ensure the test succeeded.
	validation func(t *testing.T, ctx testContext, tr *Monitor)
}

type postgresProtocolParsingSuite struct {
	suite.Suite
}

func TestPostgresMonitoring(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		suite.Run(t, new(postgresProtocolParsingSuite))
	})
}

func (s *postgresProtocolParsingSuite) TestLoadPostgresBinary() {
	t := s.T()
	for name, debug := range map[string]bool{"enabled": true, "disabled": false} {
		t.Run(name, func(t *testing.T) {
			cfg := getPostgresDefaultTestConfiguration()
			cfg.BPFDebug = debug
			setupUSMTLSMonitor(t, cfg)
		})
	}
}

func (s *postgresProtocolParsingSuite) TestDecoding() {
	t := s.T()

	serverHost := "127.0.0.1"

	serverAddress := net.JoinHostPort(serverHost, postgresPort)
	require.NoError(t, postgres.RunServer(t, serverHost, postgresPort))

	tests := []postgresParsingTestAttributes{
		{
			name: "create table",
			preMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
			},
			postMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.RunCreateQuery(t, ctx.extras)
			},
			validation: func(t *testing.T, ctx testContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.CreateTableOP: 2,
					},
				})
			},
		},
		{
			name: "insert rows in table",
			preMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				postgres.RunCreateQuery(t, ctx.extras)
			},
			postMonitorSetup: func(t *testing.T, ctx testContext) {
				// Sending 2 insert queries, each with 5 values.
				// We want to ensure we're capturing both requests.
				for i := 0; i < 2; i++ {
					postgres.RunMultiInsertQuery(t, ctx.extras, "value-1", "value-2", "value-3", "value-4", "value-5")
				}
			},
			validation: func(t *testing.T, ctx testContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.InsertOP: 4,
					},
				})
			},
		},
		{
			name: "update a row in a table",
			preMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				postgres.RunCreateQuery(t, ctx.extras)
				postgres.RunMultiInsertQuery(t, ctx.extras, "value-1")
			},
			postMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.RunUpdateQuery(t, ctx.extras)
			},
			validation: func(t *testing.T, ctx testContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.UpdateOP: 2,
					},
				})
			},
		},
		{
			name: "select",
			preMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				postgres.RunCreateQuery(t, ctx.extras)
				postgres.RunMultiInsertQuery(t, ctx.extras, "value-1")
			},
			postMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.RunSelectQuery(t, ctx.extras)
			},
			validation: func(t *testing.T, ctx testContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.SelectOP: 2,
					},
				})
			},
		},
		{
			name: "drop table",
			preMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
			},
			postMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.RunDropQuery(t, ctx.extras)
			},
			validation: func(t *testing.T, ctx testContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.DropTableOP: 2,
					},
				})
			},
		},
		{
			name: "combo - multiple operations should be captured",
			preMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
			},
			postMonitorSetup: func(t *testing.T, ctx testContext) {
				prepareTestDB(t, ctx)
				postgres.RunSelectQueryWithLimit(t, ctx.extras, 50)
				postgres.RunUpdateQuery(t, ctx.extras)
			},
			validation: func(t *testing.T, ctx testContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.SelectOP:      2,
						postgres.UpdateOP:      2,
						postgres.InsertOP:      40,
						postgres.CreateTableOP: 2,
					},
				})
			},
		},
		{
			name: "query is truncated",
			preMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
			},
			postMonitorSetup: func(t *testing.T, ctx testContext) {
				db, _ := postgres.GetCtx(ctx.extras)
				longTableName := strings.Repeat("table_", 15)
				_, err := db.Query(fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, foo TEXT)", longTableName), nil)
				require.NoError(t, err)
				_, err = db.Query(fmt.Sprintf("DROP TABLE IF EXISTS %s", longTableName), nil)
				require.NoError(t, err)
			},
			validation: func(t *testing.T, ctx testContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"table_table_table_table_table_table_table_table_tab": {
						postgres.CreateTableOP: 2,
					},
					"table_table_table_table_table_table_table_t": {
						postgres.DropTableOP: 2,
					},
				})
			},
		},
		{
			name: "too many messages in a single packet",
			preMonitorSetup: func(t *testing.T, ctx testContext) {
				postgres.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
				postgres.RunCreateQuery(t, ctx.extras)
				values := make([]string, 100)
				for i := 0; i < len(values); i++ {
					values[i] = fmt.Sprintf("value-%d", i+1)
				}
				postgres.RunMultiInsertQuery(t, ctx.extras, values...)
			},
			postMonitorSetup: func(t *testing.T, ctx testContext) {
				// Should produce 2 postgres transactions (one for the server and one for the docker proxy)
				postgres.RunSelectQueryWithLimit(t, ctx.extras, 1)
				// We should miss that query, as the response is too big.
				postgres.RunSelectQuery(t, ctx.extras)
				// Should produce 2 postgres transactions (one for the server and one for the docker proxy)
				postgres.RunSelectQueryWithLimit(t, ctx.extras, 1)
			},
			validation: func(t *testing.T, ctx testContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.SelectOP: 4,
					},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.context = testContext{
				serverPort:    postgresPort,
				targetAddress: serverAddress,
				serverAddress: serverAddress,
				extras:        map[string]interface{}{},
			}
			t.Cleanup(func() {
				db := tt.context.extras["db"].(*bun.DB)
				defer db.Close()
				taskCtx := tt.context.extras["ctx"].(context.Context)
				_, _ = db.NewDropTable().Model((*postgres.DummyTable)(nil)).Exec(taskCtx)
			})
			if tt.preMonitorSetup != nil {
				tt.preMonitorSetup(t, tt.context)
			}
			monitor := setupUSMTLSMonitor(t, getPostgresDefaultTestConfiguration())
			tt.postMonitorSetup(t, tt.context)
			tt.validation(t, tt.context, monitor)
		})
	}
}

// getPostgresInFlightEntries returns the entries in the in-flight map.
func getPostgresInFlightEntries(t *testing.T, monitor *Monitor) map[postgres.ConnTuple]postgres.EbpfTx {
	postgresInFlightMap, _, err := monitor.ebpfProgram.GetMap(postgres.InFlightMap)
	require.NoError(t, err)

	var key postgres.ConnTuple
	var value postgres.EbpfTx
	entries := make(map[postgres.ConnTuple]postgres.EbpfTx)
	iter := postgresInFlightMap.Iterate()
	for iter.Next(&key, &value) {
		entries[key] = value
	}
	return entries
}

// TestCleanupEBPFEntriesOnTermination tests that the cleanup of the eBPF entries is done when the connection
// is closed. This is important to avoid leaking resources. The test creates a TCP server, which just reads the requests
// without sending any response. The test will send a postgres request (and obviously will fail), we will verify the
// request appear in the in_flight map and then we will close the connection and verify that the entry is removed.
func (s *postgresProtocolParsingSuite) TestCleanupEBPFEntriesOnTermination() {
	t := s.T()

	// Creating the monitor
	monitor := setupUSMTLSMonitor(t, getPostgresDefaultTestConfiguration())

	wg := sync.WaitGroup{}

	// Spinning the TCP server
	const serverAddress = "127.0.0.1:5432"
	srv := testutil.NewTCPServer(serverAddress, func(conn net.Conn) {
		defer conn.Close()
		defer wg.Done()
		_, _ = conn.Read(make([]byte, 1024))
		// Verifying the entry is present in the in-flight map
		entries := getPostgresInFlightEntries(t, monitor)
		require.Len(t, entries, 1)
	}, false)
	done := make(chan struct{})
	require.NoError(t, srv.Run(done))
	t.Cleanup(func() { close(done) })

	// Encoding a dummy query.
	output := make([]byte, 0)
	query := pgproto3.Query{String: "SELECT * FROM dummy"}
	output = query.Encode(output)

	// Connecting to the server
	client, err := net.Dial("tcp", serverAddress)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	// Sending the query and waiting for the server to finish processing it
	wg.Add(1)
	_, err = client.Write(output)
	require.NoError(t, err)
	wg.Wait()

	// Closing the connection and verifying the entry is removed
	require.NoError(t, client.Close())
	entries := getPostgresInFlightEntries(t, monitor)
	require.Len(t, entries, 0)
}

func getPostgresDefaultTestConfiguration() *config.Config {
	cfg := config.New()
	cfg.EnablePostgresMonitoring = true
	cfg.MaxTrackedConnections = 1000
	return cfg
}

func prepareTestDB(t *testing.T, ctx testContext) {
	postgres.ConnectAndGetDB(t, ctx.serverAddress, ctx.extras)
	postgres.RunCreateQuery(t, ctx.extras)
	values := make([]string, 5)
	for i := 0; i < 20; i++ {
		for j := 0; j < len(values); j++ {
			values[j] = fmt.Sprintf("value-%d", i*len(values)+j+1)
		}
		postgres.RunMultiInsertQuery(t, ctx.extras, values...)
	}
}

func validatePostgres(t *testing.T, monitor *Monitor, expectedStats map[string]map[postgres.Operation]int) {
	found := make(map[string]map[postgres.Operation]int)
	require.Eventually(t, func() bool {
		postgresProtocolStats, exists := monitor.GetProtocolStats()[protocols.Postgres]
		if !exists {
			return false
		}
		// We might not have postgres stats, and it might be the expected case (to capture 0).
		currentStats := postgresProtocolStats.(map[postgres.Key]*postgres.RequestStat)
		for key, stats := range currentStats {
			if _, ok := found[key.TableName]; !ok {
				found[key.TableName] = make(map[postgres.Operation]int)
			}
			found[key.TableName][key.Operation] += stats.Count
		}
		return reflect.DeepEqual(expectedStats, found)
	}, time.Second*5, time.Millisecond*100, "Expected to find a %v stats, instead captured %v", &expectedStats, &found)
}
