// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres/ebpf"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	gotlstestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

const (
	postgresPort             = "5432"
	repeatCount              = ebpf.BufferSize / len("table_")
	createTableQuery         = "CREATE TABLE dummy (id SERIAL PRIMARY KEY, foo TEXT)"
	updateSingleValueQuery   = "UPDATE dummy SET foo = 'updated' WHERE id = 1"
	selectAllQuery           = "SELECT * FROM dummy"
	selectParameterizedQuery = "SELECT * FROM dummy WHERE id = $1"
	dropTableQuery           = "DROP TABLE IF EXISTS dummy"
	deleteTableQuery         = "DELETE FROM dummy WHERE id = 1"
	alterTableQuery          = "ALTER TABLE dummy ADD test VARCHAR(255);"
	truncateTableQuery       = "TRUNCATE TABLE dummy"
	showQuery                = "SHOW search_path"
)

var (
	longCreateQuery = fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, foo TEXT)", strings.Repeat("table_", repeatCount))
	longDropeQuery  = fmt.Sprintf("DROP TABLE IF EXISTS %s", strings.Repeat("table_", repeatCount))
)

func createInsertQuery(values ...string) string {
	return fmt.Sprintf("INSERT INTO dummy (foo) VALUES ('%s')", strings.Join(values, "'), ('"))
}

func generateTestValues(startingIndex, count int) []string {
	values := make([]string, count)
	for i := 0; i < count; i++ {
		values[i] = fmt.Sprintf("value-%d", startingIndex+i)
	}
	return values
}

func generateSelectLimitQuery(limit int) string {
	return fmt.Sprintf("SELECT * FROM dummy limit %d", limit)
}

// pgTestContext shares the context of a given test.
// It contains common variable used by all tests, and allows extending the context dynamically by setting more
// attributes to the `extras` map.
type pgTestContext struct {
	// The address of the server to listen on.
	serverAddress string
	// The port to listen on.
	serverPort string
	// The address for the client to communicate with.
	targetAddress string
	// A dynamic map that allows extending the context easily between phases of the test.
	extras map[string]interface{}
}

// postgresParsingTestAttributes holds all attributes a single postgres parsing test should have.
type postgresParsingTestAttributes struct {
	// The name of the test.
	name string
	// Specific test context, allows to share states among different phases of the test.
	context pgTestContext
	// Allows to do any preparation without traffic being captured by the monitor.
	preMonitorSetup func(t *testing.T, ctx pgTestContext)
	// All traffic here will be captured by the monitor.
	postMonitorSetup func(t *testing.T, ctx pgTestContext)
	// A validation method ensure the test succeeded.
	validation func(t *testing.T, ctx pgTestContext, tr *Monitor)
}

type postgresProtocolParsingSuite struct {
	suite.Suite
}

func TestPostgresMonitoring(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	if !prebuilt.IsDeprecated() {
		modes = append(modes, ebpftest.Prebuilt)
	}
	ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
		suite.Run(t, new(postgresProtocolParsingSuite))
	})
}

func (s *postgresProtocolParsingSuite) TestLoadPostgresBinary() {
	t := s.T()
	for name, debug := range map[string]bool{"enabled": true, "disabled": false} {
		t.Run(name, func(t *testing.T) {
			cfg := getPostgresDefaultTestConfiguration(protocolsUtils.TLSDisabled)
			cfg.BPFDebug = debug
			setupUSMTLSMonitor(t, cfg)
		})
	}
}

func (s *postgresProtocolParsingSuite) TestDecoding() {
	t := s.T()

	tests := []struct {
		name  string
		isTLS bool
	}{
		{
			name:  "with TLS",
			isTLS: true,
		},
		{
			name:  "without TLS",
			isTLS: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.isTLS && !gotlstestutil.GoTLSSupported(t, config.New()) {
				t.Skip("GoTLS not supported for this setup")
			}
			testDecoding(t, tt.isTLS)
		})
	}
}

// waitForPostgresServer verifies that the postgres server is up and running.
// It tries to connect to the server until it succeeds or the timeout is reached.
// We need that function (and cannot relay on the RunServer method) as the target regex is being logged a couple os
// milliseconds before the server is actually ready to accept connections.
func waitForPostgresServer(t *testing.T, serverAddress string, enableTLS bool) {
	pgClient, err := postgres.NewPGXClient(postgres.ConnectionOptions{
		ServerAddress: serverAddress,
		EnableTLS:     enableTLS,
	})
	require.NoError(t, err)
	defer pgClient.Close()
	require.Eventually(t, func() bool {
		return pgClient.Ping() == nil
	}, 5*time.Second, 100*time.Millisecond, "couldn't connect to postgres server")
}

func testDecoding(t *testing.T, isTLS bool) {
	serverHost := "127.0.0.1"

	serverAddress := net.JoinHostPort(serverHost, postgresPort)
	require.NoError(t, postgres.RunServer(t, serverHost, postgresPort, isTLS))
	// Verifies that the postgres server is up and running.
	// It tries to connect to the server until it succeeds or the timeout is reached.
	// We need that function (and cannot relay on the RunServer method) as the target regex is being logged a couple os
	// milliseconds before the server is actually ready to accept connections.
	waitForPostgresServer(t, serverAddress, isTLS)

	// With non-TLS, we need to double the stats since we use Docker and the
	// packets are seen twice. This is not needed in the TLS case since there
	// the data comes from uprobes on the binary.
	adjustCount := func(count int) int {
		if isTLS {
			return count
		}

		return count * 2
	}

	monitor := setupUSMTLSMonitor(t, getPostgresDefaultTestConfiguration(isTLS))
	if isTLS {
		utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, os.Getpid(), utils.ManualTracingFallbackEnabled)
	}

	tests := []postgresParsingTestAttributes{
		{
			name: "create table",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				require.NoError(t, pg.RunQuery(createTableQuery))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.CreateTableOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		{
			name: "insert rows in table",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunQuery(createTableQuery))
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				// Sending 2 insert queries, each with 5 values.
				// We want to ensure we're capturing both requests.
				for i := 0; i < 2; i++ {
					require.NoError(t, pg.RunQuery(createInsertQuery(generateTestValues(5*i, 5*(1+i))...)))
				}
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.InsertOP: adjustCount(2),
					},
				}, isTLS)
			},
		},
		{
			name: "update a row in a table",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunQuery(createTableQuery))
				require.NoError(t, pg.RunQuery(createInsertQuery("value-1")))
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				require.NoError(t, pg.RunQuery(updateSingleValueQuery))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.UpdateOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		{
			name: "select",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunQuery(createTableQuery))
				require.NoError(t, pg.RunQuery(createInsertQuery("value-1")))
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				require.NoError(t, pg.RunQuery(selectAllQuery))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.SelectOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		{
			name: "delete row from table",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunQuery(createTableQuery))
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				require.NoError(t, pg.RunQuery(deleteTableQuery))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.DeleteTableOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		{
			name: "alter command",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunQuery(createTableQuery))
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				require.NoError(t, pg.RunQuery(alterTableQuery))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.AlterTableOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		{
			name: "truncate operation",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunQuery(createTableQuery))
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				require.NoError(t, pg.RunQuery(truncateTableQuery))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.TruncateTableOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		{
			name: "drop table",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				require.NoError(t, pg.RunQuery(dropTableQuery))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.DropTableOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		{
			name: "combo - multiple operations should be captured",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunQuery(createTableQuery))
				for i := 0; i < 20; i++ {
					require.NoError(t, pg.RunQuery(createInsertQuery(generateTestValues(i*5, 5)...)))
				}
				require.NoError(t, pg.RunQuery(generateSelectLimitQuery(50)))
				require.NoError(t, pg.RunQuery(updateSingleValueQuery))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.SelectOP:      adjustCount(1),
						postgres.UpdateOP:      adjustCount(1),
						postgres.InsertOP:      adjustCount(20),
						postgres.CreateTableOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		{
			name: "query is truncated",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				require.NoError(t, pg.RunQuery(longCreateQuery))
				require.NoError(t, pg.RunQuery(longDropeQuery))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					getTruncatedTableName(longCreateQuery, 13): {
						postgres.CreateTableOP: adjustCount(1),
					},
					getTruncatedTableName(longDropeQuery, 21): {
						postgres.DropTableOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		{
			name: "too many messages in a single packet",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunQuery(createTableQuery))
				require.NoError(t, pg.RunQuery(createInsertQuery(generateTestValues(0, 100)...)))
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				// Should produce 2 postgres transactions (one for the server and one for the docker proxy)
				require.NoError(t, pg.RunQuery(generateSelectLimitQuery(1)))
				require.NoError(t, pg.RunQuery(selectAllQuery))
				// Should produce 2 postgres transactions (one for the server and one for the docker proxy)
				require.NoError(t, pg.RunQuery(generateSelectLimitQuery(1)))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.SelectOP: adjustCount(2),
					},
				}, isTLS)
			},
		},
		{
			name: "show command",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunQuery(createTableQuery))
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				require.NoError(t, pg.RunQuery(showQuery))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"search_path": {
						postgres.ShowOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		// This test validates that the sql transaction is not supported.
		{
			name: "transaction",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg

				tx, err := pg.Begin()
				require.NoError(t, err)
				require.NoError(t, pg.RunQuery(createTableQuery))
				require.NoError(t, pg.Commit(tx))
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)

				tx, err := pg.Begin()
				require.NoError(t, err)
				require.NoError(t, pg.RunQueryTX(tx, selectAllQuery))
				require.NoError(t, pg.Commit(tx))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"UNKNOWN": {
						postgres.UnknownOP: adjustCount(2),
					},
					"dummy": {
						postgres.SelectOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		{
			name: "batched queries",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunQuery(createTableQuery))

			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)

				require.NoError(t, pg.SendBatch(createInsertQuery("value-1"), selectAllQuery))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{
					"dummy": {
						postgres.InsertOP: adjustCount(1),
					},
				}, isTLS)
			},
		},
		// This test validates that parameterized queries are currently not supported.
		{
			name: "parameterized select",
			preMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg, err := postgres.NewPGXClient(postgres.ConnectionOptions{
					ServerAddress: ctx.serverAddress,
					EnableTLS:     isTLS,
				})
				require.NoError(t, err)
				require.NoError(t, pg.Ping())
				ctx.extras["pg"] = pg
				require.NoError(t, pg.RunQuery(createTableQuery))
				require.NoError(t, pg.RunQuery(createInsertQuery("value-1")))
			},
			postMonitorSetup: func(t *testing.T, ctx pgTestContext) {
				pg := ctx.extras["pg"].(*postgres.PGXClient)
				require.NoError(t, pg.RunQuery(selectParameterizedQuery, "value-1"))
			},
			validation: func(t *testing.T, _ pgTestContext, monitor *Monitor) {
				validatePostgres(t, monitor, map[string]map[postgres.Operation]int{}, isTLS)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.context = pgTestContext{
				serverPort:    postgresPort,
				targetAddress: serverAddress,
				serverAddress: serverAddress,
				extras:        map[string]interface{}{},
			}
			t.Cleanup(func() {
				pgEntry, ok := tt.context.extras["pg"]
				if !ok {
					return
				}
				pg := pgEntry.(*postgres.PGXClient)
				defer pg.Close()
				_ = pg.RunQuery(dropTableQuery)
				cleanProtocolMaps(t, "postgres", monitor.ebpfProgram.Manager.Manager)
			})
			require.NoError(t, monitor.Pause())
			if tt.preMonitorSetup != nil {
				tt.preMonitorSetup(t, tt.context)
			}
			require.NoError(t, monitor.Resume())

			obj, ok := tt.context.extras["pg"]
			require.True(t, ok)
			pgClient := obj.(*postgres.PGXClient)
			// Since we cannot classify 'Parse' message, we need to send a harmless message that we know how
			// to classify to ensure the monitor is able to process the messages.
			// That's a workaround until we can classify the 'Parse' message.
			require.NoError(t, pgClient.Ping())
			tt.postMonitorSetup(t, tt.context)
			require.NoError(t, monitor.Pause())
			tt.validation(t, tt.context, monitor)
		})
	}
}

// getTruncatedTableName returns the truncated table name by reducing the operation and extracting the remaining
// table name by the current max buffer size.
func getTruncatedTableName(query string, tableNameIndex int) string {
	return query[tableNameIndex:ebpf.BufferSize]
}

// getPostgresInFlightEntries returns the entries in the in-flight map.
func getPostgresInFlightEntries(t *testing.T, monitor *Monitor) map[ebpf.ConnTuple]ebpf.EbpfTx {
	postgresInFlightMap, _, err := monitor.ebpfProgram.GetMap(postgres.InFlightMap)
	require.NoError(t, err)

	var key ebpf.ConnTuple
	var value ebpf.EbpfTx
	entries := make(map[ebpf.ConnTuple]ebpf.EbpfTx)
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
	monitor := setupUSMTLSMonitor(t, getPostgresDefaultTestConfiguration(protocolsUtils.TLSDisabled))

	wg := sync.WaitGroup{}

	// Spinning the TCP server
	const serverAddress = "127.0.0.1:5433" // Using a different port than 5432 to avoid errors like "address already in use"
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
	var err error
	output, err = query.Encode(output)
	require.NoError(t, err)

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

func getPostgresDefaultTestConfiguration(enableTLS bool) *config.Config {
	cfg := config.New()
	cfg.EnablePostgresMonitoring = true
	cfg.MaxTrackedConnections = 1000
	cfg.EnableGoTLSSupport = enableTLS
	// If GO TLS is enabled, we need to allow self traffic to be captured.
	// If GO TLS is disabled, the value is irrelevant.
	cfg.GoTLSExcludeSelf = false
	cfg.BypassEnabled = true
	return cfg
}

func validatePostgres(t *testing.T, monitor *Monitor, expectedStats map[string]map[postgres.Operation]int, tls bool) {
	found := make(map[string]map[postgres.Operation]int)
	require.Eventually(t, func() bool {
		postgresProtocolStats, exists := monitor.GetProtocolStats()[protocols.Postgres]
		if !exists {
			return false
		}
		// We might not have postgres stats, and it might be the expected case (to capture 0).
		currentStats := postgresProtocolStats.(map[postgres.Key]*postgres.RequestStat)
		for key, stats := range currentStats {
			hasTLSTag := stats.StaticTags&network.ConnTagGo != 0
			if hasTLSTag != tls {
				continue
			}
			if _, ok := found[key.Parameters]; !ok {
				found[key.Parameters] = make(map[postgres.Operation]int)
			}
			found[key.Parameters][key.Operation] += stats.Count
		}
		return reflect.DeepEqual(expectedStats, found)
	}, time.Second*5, time.Millisecond*100, "Expected to find a %v stats, instead captured %v", &expectedStats, &found)
}

func (s *postgresProtocolParsingSuite) TestExtractParameters() {
	t := s.T()

	units := []struct {
		name     string
		expected string
		event    ebpf.EbpfEvent
	}{
		{
			name:     "query_size longer than the actual length of the content",
			expected: "version and status",
			event: ebpf.EbpfEvent{
				Tx: ebpf.EbpfTx{
					Request_fragment:    createFragment([]byte("SHOW version and status")),
					Original_query_size: 64,
				},
			},
		},
		{
			name:     "query_size shorter than the actual length of the content",
			expected: "param1 param2",
			event: ebpf.EbpfEvent{
				Tx: ebpf.EbpfTx{
					Request_fragment:    createFragment([]byte("SHOW param1 param2 param3")),
					Original_query_size: 18,
				},
			},
		},
		{
			name:     "the query has no parameters",
			expected: postgres.EmptyParameters,
			event: ebpf.EbpfEvent{
				Tx: ebpf.EbpfTx{
					Request_fragment:    createFragment([]byte("SHOW ")),
					Original_query_size: 10,
				},
			},
		},
		{
			name:     "command has trailing zeros",
			expected: "param",
			event: ebpf.EbpfEvent{
				Tx: ebpf.EbpfTx{
					Request_fragment:    [ebpf.BufferSize]byte{'S', 'H', 'O', 'W', ' ', 'p', 'a', 'r', 'a', 'm', 0, 0, 0},
					Original_query_size: 13,
				},
			},
		},
		{
			name:     "malformed command with wrong query_size",
			expected: postgres.EmptyParameters,
			event: ebpf.EbpfEvent{
				Tx: ebpf.EbpfTx{
					Request_fragment:    [ebpf.BufferSize]byte{'S', 'H', 'O', 'W', ' ', 0, 0, 'a', ' ', 'b', 'c', 0, 0, 0},
					Original_query_size: 14,
				},
			},
		},
		{
			name:     "empty parameters with spaces and nils",
			expected: postgres.EmptyParameters,
			event: ebpf.EbpfEvent{
				Tx: ebpf.EbpfTx{
					Request_fragment:    [ebpf.BufferSize]byte{'S', 'H', 'O', 'W', ' ', 0, ' ', 0, ' ', 0, 0, 0},
					Original_query_size: 12,
				},
			},
		},
		{
			name:     "parameters with control codes only",
			expected: "\x01\x02\x03\x04\x05",
			event: ebpf.EbpfEvent{
				Tx: ebpf.EbpfTx{
					Request_fragment:    [ebpf.BufferSize]byte{'S', 'H', 'O', 'W', ' ', 1, 2, 3, 4, 5},
					Original_query_size: 10,
				},
			},
		},
	}
	for _, unit := range units {
		t.Run(unit.name, func(t *testing.T) {
			e := postgres.NewEventWrapper(&unit.event)
			require.NotNil(t, e)
			e.Operation()
			require.Equal(t, unit.expected, e.Parameters())
		})
	}
}

func createFragment(fragment []byte) [ebpf.BufferSize]byte {
	var b [ebpf.BufferSize]byte
	copy(b[:], fragment)
	return b
}
