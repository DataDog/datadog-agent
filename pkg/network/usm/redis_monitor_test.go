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

	"github.com/stretchr/testify/assert"
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
	skipIfKernelNotSupported(t, redis.MinimumKernelVersion, "Redis")

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
		trackResources  bool
	}{
		{
			name:            "with TLS with RESP2",
			isTLS:           true,
			protocolVersion: 2,
			trackResources:  true,
		},
		{
			name:            "with TLS with RESP3",
			isTLS:           true,
			protocolVersion: 3,
			trackResources:  true,
		},
		{
			name:            "without TLS with RESP2",
			isTLS:           false,
			protocolVersion: 2,
			trackResources:  true,
		},
		{
			name:            "without TLS with RESP3",
			isTLS:           false,
			protocolVersion: 3,
			trackResources:  true,
		},
		{
			name:            "without TLS with RESP2 and without resource tracking",
			isTLS:           false,
			protocolVersion: 2,
			trackResources:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.isTLS && !gotlstestutil.GoTLSSupported(t, NewUSMEmptyConfig()) {
				t.Skip("GoTLS not supported for this setup")
			}
			testRedisDecoding(t, tt.isTLS, tt.protocolVersion, tt.trackResources)
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
	}, 10*time.Second, 100*time.Millisecond, "couldn't connect to redis server")
}

func testRedisDecoding(t *testing.T, isTLS bool, version int, trackResources bool) {
	serverHost := "127.0.0.1"
	serverAddress := net.JoinHostPort(serverHost, redisPort)
	require.NoError(t, redis.RunServer(t, serverHost, redisPort, isTLS))
	waitForRedisServer(t, serverAddress, isTLS, version)

	cfg := getRedisDefaultTestConfiguration(isTLS)
	cfg.RedisTrackResources = trackResources
	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
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
				keyName := ""
				if trackResources {
					keyName = "test-key"
				}
				expected := map[string]map[redis.CommandType]int{
					keyName: {
						redis.GetCommand: adjustCount(1),
						redis.SetCommand: adjustCount(1),
					},
				}
				// Add warmup PING - merge into same key if trackResources=false
				if keyName == "" {
					expected[""][redis.PingCommand] = adjustCount(1)
				} else {
					expected[""] = map[redis.CommandType]int{
						redis.PingCommand: adjustCount(1),
					}
				}
				validateRedis(t, monitor, expected, isTLS)
			},
		},
	}

	// PING tests should only run when trackResources=false since PING doesn't have keys
	if !trackResources {
		tests = append(tests, redisParsingTestAttributes{
			name: "ping command",
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
				// Execute multiple PING commands
				for i := 0; i < 5; i++ {
					require.NoError(t, redisClient.Ping(context.Background()).Err())
				}
			},
			validation: func(t *testing.T, _ redisTestContext, monitor *Monitor) {
				// PING doesn't have a key, so keyName is always empty
				// Count includes: 1 PING from line 286 + 5 PINGs from postMonitorSetup = 6 total
				validateRedis(t, monitor, map[string]map[redis.CommandType]int{
					"": {
						redis.PingCommand: adjustCount(6),
					},
				}, isTLS)
			},
		})

		tests = append(tests, redisParsingTestAttributes{
			name: "ping command with message (bulk string response)",
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
				// PING with message argument returns bulk string instead of simple string
				// This tests that all RESP response types are accepted
				for i := 0; i < 3; i++ {
					result, err := redisClient.Do(context.Background(), "PING", "hello").Result()
					require.NoError(t, err)
					require.Equal(t, "hello", result)
				}
			},
			validation: func(t *testing.T, _ redisTestContext, monitor *Monitor) {
				// PING doesn't have a key, so keyName is always empty
				// Count includes: 1 PING from line 287 + 3 PINGs with message from postMonitorSetup = 4 total
				validateRedis(t, monitor, map[string]map[redis.CommandType]int{
					"": {
						redis.PingCommand: adjustCount(4),
					},
				}, isTLS)
			},
		})
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

			// Give monitor time to fully start before sending traffic
			time.Sleep(100 * time.Millisecond)

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
	cfg := NewUSMEmptyConfig()
	cfg.EnableRedisMonitoring = true
	cfg.RedisTrackResources = true // Enable resource tracking for tests
	cfg.MaxTrackedConnections = 1000
	cfg.EnableGoTLSSupport = enableTLS
	cfg.GoTLSExcludeSelf = false
	cfg.BypassEnabled = true
	return cfg
}

func validateRedis(t *testing.T, monitor *Monitor, expectedStats map[string]map[redis.CommandType]int, tls bool) {
	found := make(map[string]map[redis.CommandType]int)
	assert.Eventually(t, func() bool {
		statsObj, cleaners := monitor.GetProtocolStats()
		defer cleaners()
		redisProtocolStats, exists := statsObj[protocols.Redis]
		if !exists {
			return false
		}
		currentStats := redisProtocolStats.(map[redis.Key]*redis.RequestStats)

		for key, stats := range currentStats {
			// Check all error states for TLS tag
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

			keyName := ""
			if key.KeyName != nil {
				keyName = key.KeyName.Get()
			}

			if _, ok := found[keyName]; !ok {
				found[keyName] = make(map[redis.CommandType]int)
			}
			for _, stat := range stats.ErrorToStats {
				found[keyName][key.Command] += stat.Count
			}
		}
		return reflect.DeepEqual(expectedStats, found)
	}, time.Second*5, time.Millisecond*100, "Expected to find %v stats, instead captured %v", &expectedStats, &found)
}
