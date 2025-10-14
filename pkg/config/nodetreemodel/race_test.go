// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package nodetreemodel

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// TestConcurrentGetBoolDataRace reproduces a data race in nodetreemodel where
// concurrent GetBool() calls trigger writes to c.root while other goroutines
// are reading from it, all while holding only read locks (RLock).
//
// This test reproduces the exact race condition seen in CI test failures:
// TestRaceFlushVersusParsePacket in pkg/serverless/metrics/metric_test.go
//
// The race occurs when:
// 1. SetTestOnlyDynamicSchema(true) is enabled (used by configmock.New in tests)
// 2. Multiple goroutines concurrently call GetBool/GetString/GetInt
// 3. One goroutine triggers: GetBool() → checkKnownKey() → maybeRebuild() →
//    buildSchema() → mergeAllLayers() which WRITES to c.root at line 483
// 4. Another goroutine is reading: GetBool() → getNodeValue() which READS
//    from c.root at line 141
// 5. Both hold READ locks (RLock), but one performs a WRITE = DATA RACE
//
// Root cause: Commit 05e00bf3dd1 (March 28, 2025) added maybeRebuild() which
// is called from checkKnownKey() while the caller holds only a read lock.
//
// Run with: go test -race -run TestConcurrentGetBoolDataRace ./pkg/config/nodetreemodel -v
func TestConcurrentGetBoolDataRace(t *testing.T) {
	// Create a config exactly as configmock.New() does
	cfg := NewNodeTreeConfig("test", "DD", strings.NewReplacer(".", "_"))

	// Set up a few keys with defaults (mimics what real tests do)
	cfg.SetKnown("telemetry.enabled")
	cfg.SetKnown("use_v2_api.series")
	cfg.SetDefault("telemetry.enabled", true)
	cfg.SetDefault("use_v2_api.series", false)

	// Build schema and enable dynamic schema mode
	// This is the critical step that enables the buggy code path
	cfg.BuildSchema()
	cfg.SetTestOnlyDynamicSchema(true)

	// Now simulate concurrent config access as happens in production tests
	var wg sync.WaitGroup
	duration := 2 * time.Second
	stop := time.After(duration)

	// Goroutine 1: Simulates pkg/serializer.SendIterableSeries() reading config
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				// This can trigger maybeRebuild() which WRITES to c.root
				_ = cfg.GetBool("use_v2_api.series")
			}
		}
	}()

	// Goroutine 2: Simulates pkg/aggregator.TimeSampler.sendTelemetry() reading config
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				// This calls getNodeValue() which READS from c.root
				_ = cfg.GetBool("telemetry.enabled")
			}
		}
	}()

	// Goroutine 3: Add a third concurrent reader to increase race likelihood
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = cfg.GetBool("use_v2_api.series")
			}
		}
	}()

	wg.Wait()

	// If running with -race flag, the race detector will report:
	// "WARNING: DATA RACE
	//  Write at 0x... by goroutine X:
	//    pkg/config/nodetreemodel.(*ntmConfig).mergeAllLayers()
	//        config.go:483
	//  Previous read at 0x... by goroutine Y:
	//    pkg/config/nodetreemodel.(*ntmConfig).getNodeValue()
	//        getter.go:141"
}
