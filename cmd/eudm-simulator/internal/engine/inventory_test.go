// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestInventoryReplayRebasesObservedSnapshotsWithoutRestartingAgent(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 123456789, time.UTC)
	const startupMS = -123456
	for _, capturedAt := range []int64{int64(5*time.Second + 987654321), int64(10*time.Minute + 123456789), int64(30 * time.Minute)} {
		for _, agent := range []bool{false, true} {
			value := &telemetrycapture.Inventory{Timestamp: capturedAt}
			if agent {
				value.Agent = &telemetrycapture.AgentInventoryMetadata{AgentStartupTimeMS: startupMS}
			}
			rebase(&telemetry.Sample{Inventory: value}, start, 1, 1)
			if value.Timestamp != start.UnixNano()+capturedAt {
				t.Fatal("inventory timestamp lost its captured offset or fractional time")
			}
			if agent && value.Agent.AgentStartupTimeMS != start.UnixMilli()+startupMS {
				t.Fatal("a later observation changed the Agent startup time")
			}
		}
	}
}
