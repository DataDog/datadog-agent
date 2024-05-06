// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/uptrace/bun"

	"github.com/DataDog/datadog-agent/pkg/network/config"
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
	// The test body
	testBody func(t *testing.T, ctx testContext, monitor *Monitor)
	// Cleaning test resources if needed.
	teardown func(t *testing.T, ctx testContext)
	// Configuration for the monitor object
	configuration func() *config.Config
}

type postgresProtocolParsingSuite struct {
	suite.Suite
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
	for i := 0; i < 20; i++ {
		postgres.RunInsertMultiQueryWithString(t, fmt.Sprintf("value-%d-", i+1), 5, ctx.extras)
	}
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

func getAndValidatePostgresStats(t *testing.T, monitor *Monitor, expectedStatsCount int) map[postgres.Key]*postgres.RequestStat {
	statsCount := PrintableInt(0)
	postgresStats := make(map[postgres.Key]*postgres.RequestStat)
	require.Eventually(t, func() bool {
		protocolStats := monitor.GetProtocolStats()
		postgresProtocolStats, exists := protocolStats[protocols.Postgres]
		// We might not have postgres stats, and it might be the expected case (to capture 0).
		if exists {
			currentStats := postgresProtocolStats.(map[postgres.Key]*postgres.RequestStat)
			for key, stats := range currentStats {
				prevStats, ok := postgresStats[key]
				if ok && prevStats != nil {
					prevStats.CombineWith(stats)
				} else {
					postgresStats[key] = currentStats[key]
				}
			}
		}
		statsCount = PrintableInt(len(postgresStats))
		return expectedStatsCount == len(postgresStats)
	}, time.Second*5, time.Millisecond*100, "Expected to find a %d stats, instead captured %v", expectedStatsCount, &statsCount)
	return postgresStats
}

func (s *postgresProtocolParsingSuite) TestDecoding() {
	t := s.T()

	//clientHost := "localhost"
	targetHost := "127.0.0.1"
	serverHost := "127.0.0.1"

	postgresTeardown := func(t *testing.T, ctx testContext) {
		db := ctx.extras["db"].(*bun.DB)
		defer db.Close()
		taskCtx := ctx.extras["ctx"].(context.Context)
		_, _ = db.NewDropTable().Model((*postgres.DummyTable)(nil)).Exec(taskCtx)
	}

	serverAddress := net.JoinHostPort(serverHost, postgresPort)
	targetAddress := net.JoinHostPort(targetHost, postgresPort)
	require.NoError(t, postgres.RunServer(t, serverHost, postgresPort))

	//defaultDialer := &net.Dialer{
	//	LocalAddr: &net.TCPAddr{
	//		IP: net.ParseIP(clientHost),
	//	},
	//}

	tests := []postgresParsingTestAttributes{
		{
			name: "Sanity - simple select",
			context: testContext{
				serverPort:    postgresPort,
				targetAddress: targetAddress,
				serverAddress: serverAddress,
				extras:        map[string]interface{}{},
			},
			testBody: func(t *testing.T, ctx testContext, monitor *Monitor) {
				prepareTestDB(t, ctx)
				postgres.RunSelectQuery(t, ctx.extras, 50)
				postgres.RunUpdateQuery(t, ctx.extras)
				// 1 create, 1 inserts, 1 select, 1 update
				stats := getAndValidatePostgresStats(t, monitor, 2*(1+1+1+1))
				for key, stat := range stats {
					require.Equal(t, "dummy", key.TableName)
					switch key.Operation {
					case "SELECT":
						require.Equal(t, 1, stat.Count)
					case "UPDATE":
						require.Equal(t, 1, stat.Count)
					case "INSERT":
						require.Equal(t, 20, stat.Count)
					case "CREATE":
						require.Equal(t, 1, stat.Count)
					}
				}
			},
			teardown:      postgresTeardown,
			configuration: getPostgresDefaultTestConfiguration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.teardown != nil {
				t.Cleanup(func() {
					tt.teardown(t, tt.context)
				})
			}
			monitor := setupUSMTLSMonitor(t, tt.configuration())
			tt.testBody(t, tt.context, monitor)
		})
	}
}
