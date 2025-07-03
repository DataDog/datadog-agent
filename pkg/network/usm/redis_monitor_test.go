// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"context"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/redis"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	ebpftls "github.com/DataDog/datadog-agent/pkg/network/protocols/tls"
	gotlstestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/consts"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"

	redisv9 "github.com/redis/go-redis/v9"
)

const (
	redisPort = "6379"
)

type redisTestContext struct {
	serverAddress string
	serverPort    string
	targetAddress string
	extras        map[string]interface{}
}

type redisParsingTestAttributes struct {
	name             string
	context          redisTestContext
	preMonitorSetup  func(t *testing.T, ctx redisTestContext)
	postMonitorSetup func(t *testing.T, ctx redisTestContext)
	validation       func(t *testing.T, ctx redisTestContext, tr *Monitor)
}

type redisProtocolParsingSuite struct {
	suite.Suite
}

func TestRedisMonitoring(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	ebpftest.TestBuildModes(t, usmtestutil.SupportedBuildModes(), "", func(t *testing.T) {
		suite.Run(t, new(redisProtocolParsingSuite))
	})
}

func (s *redisProtocolParsingSuite) TestLoadRedisBinary() {
	t := s.T()
	for name, debug := range map[string]bool{"enabled": true, "disabled": false} {
		t.Run(name, func(t *testing.T) {
			cfg := getRedisDefaultTestConfiguration(protocolsUtils.TLSDisabled)
			cfg.BPFDebug = debug
			setupUSMTLSMonitor(t, cfg, useExistingConsumer)
		})
	}
}

func (s *redisProtocolParsingSuite) TestDecoding() {
	t := s.T()

	tests := []struct {
		name            string
		isTLS           bool
		protocolVersion int
	}{
		{
			name:            "with TLS with RESP2",
			isTLS:           true,
			protocolVersion: 2,
		},
		{
			name:            "with TLS with RESP3",
			isTLS:           true,
			protocolVersion: 3,
		},
		{
			name:            "without TLS with RESP2",
			isTLS:           false,
			protocolVersion: 2,
		},
		{
			name:            "without TLS with RESP3",
			isTLS:           false,
			protocolVersion: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.isTLS && !gotlstestutil.GoTLSSupported(t, utils.NewUSMEmptyConfig()) {
				t.Skip("GoTLS not supported for this setup")
			}
			testRedisDecoding(t, tt.isTLS, tt.protocolVersion)
		})
	}
}

func waitForRedisServer(t *testing.T, serverAddress string, enableTLS bool, version int) {
	dialer := &net.Dialer{}
	redisClient, err := redis.NewClient(serverAddress, dialer, enableTLS, version)
	require.NoError(t, err)
	require.NotNil(t, redisClient)
	defer redisClient.Close()
	require.Eventually(t, func() bool {
		return redisClient.Ping(context.Background()).Err() == nil
	}, 5*time.Second, 100*time.Millisecond, "couldn't connect to redis server")
}

func testRedisDecoding(t *testing.T, isTLS bool, version int) {
	serverHost := "127.0.0.1"
	serverAddress := net.JoinHostPort(serverHost, redisPort)
	require.NoError(t, redis.RunServer(t, serverHost, redisPort, isTLS))
	waitForRedisServer(t, serverAddress, isTLS, version)

	monitor := setupUSMTLSMonitor(t, getRedisDefaultTestConfiguration(isTLS), useExistingConsumer)
	if isTLS {
		utils.WaitForProgramsToBeTraced(t, consts.USMModuleName, GoTLSAttacherName, os.Getpid(), utils.ManualTracingFallbackEnabled)
	}

	// With non-TLS, we need to double the stats since we use Docker and the
	// packets are seen twice. This is not needed in the TLS case since there
	// the data comes from uprobes on the binary.
	adjustCount := func(count int) int {
		if isTLS {
			return count
		}

		return count * 2
	}

	tests := []redisParsingTestAttributes{
		{
			name: "set and get commands",
			preMonitorSetup: func(t *testing.T, ctx redisTestContext) {
				dialer := &net.Dialer{}
				redisClient, err := redis.NewClient(ctx.serverAddress, dialer, isTLS, version)
				require.NoError(t, err)
				require.NotNil(t, redisClient)
				require.NoError(t, redisClient.Ping(context.Background()).Err())
				ctx.extras["redis"] = redisClient
			},
			postMonitorSetup: func(t *testing.T, ctx redisTestContext) {
				redisClient := ctx.extras["redis"].(*redisv9.Client)
				require.NoError(t, redisClient.Set(context.Background(), "test-key", "test-value", 0).Err())
				val, err := redisClient.Get(context.Background(), "test-key").Result()
				require.NoError(t, err)
				require.Equal(t, "test-value", val)
			},
			validation: func(t *testing.T, _ redisTestContext, monitor *Monitor) {
				validateRedis(t, monitor, map[string]map[redis.CommandType]int{
					"test-key": {
						redis.GetCommand: adjustCount(1),
						redis.SetCommand: adjustCount(1),
					},
				}, isTLS)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.context = redisTestContext{
				serverPort:    redisPort,
				targetAddress: serverAddress,
				serverAddress: serverAddress,
				extras:        map[string]interface{}{},
			}
			t.Cleanup(func() {
				redisEntry, ok := tt.context.extras["redis"]
				if !ok {
					return
				}
				redisClient := redisEntry.(*redisv9.Client)
				defer redisClient.Close()
				cleanProtocolMaps(t, "redis", monitor.ebpfProgram.Manager.Manager)
			})
			require.NoError(t, monitor.Pause())
			if tt.preMonitorSetup != nil {
				tt.preMonitorSetup(t, tt.context)
			}
			require.NoError(t, monitor.Resume())

			obj, ok := tt.context.extras["redis"]
			require.True(t, ok)
			redisClient := obj.(*redisv9.Client)
			require.NoError(t, redisClient.Ping(context.Background()).Err())
			tt.postMonitorSetup(t, tt.context)
			require.NoError(t, monitor.Pause())
			tt.validation(t, tt.context, monitor)
		})
	}
}

func getRedisDefaultTestConfiguration(enableTLS bool) *config.Config {
	cfg := utils.NewUSMEmptyConfig()
	cfg.EnableRedisMonitoring = true
	cfg.MaxTrackedConnections = 1000
	cfg.EnableGoTLSSupport = enableTLS
	cfg.GoTLSExcludeSelf = false
	cfg.BypassEnabled = true
	return cfg
}

func validateRedis(t *testing.T, monitor *Monitor, expectedStats map[string]map[redis.CommandType]int, tls bool) {
	found := make(map[string]map[redis.CommandType]int)
	require.Eventually(t, func() bool {
		statsObj, cleaners := monitor.GetProtocolStats()
		t.Cleanup(cleaners)
		redisProtocolStats, exists := statsObj[protocols.Redis]
		if !exists {
			return false
		}
		currentStats := redisProtocolStats.(map[redis.Key]*redis.RequestStats)
		for key, stats := range currentStats {
			// Check all error states for TLS tag and sum counts
			var hasTLSTag bool
			for _, stat := range stats.ErrorToStats {
				if stat.StaticTags&ebpftls.ConnTagGo != 0 {
					hasTLSTag = true
					break
				}
			}
			if hasTLSTag != tls {
				continue
			}
			if _, ok := found[key.KeyName.Get()]; !ok {
				found[key.KeyName.Get()] = make(map[redis.CommandType]int)
			}
			for _, stat := range stats.ErrorToStats {
				found[key.KeyName.Get()][key.Command] += stat.Count
			}
		}
		return reflect.DeepEqual(expectedStats, found)
	}, time.Second*5, time.Millisecond*100, "Expected to find %v stats, instead captured %v", &expectedStats, &found)
}
