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

func TestInventoryReplayAdvancesSnapshotsWithoutRestartingAgent(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 123456789, time.UTC)
	const capturedAt = int64(5*time.Second + 987654321)
	const startupMS = -123456
	for _, shift := range []time.Duration{0, 10 * time.Minute, 30 * time.Minute} {
		for _, agent := range []bool{false, true} {
			value := &telemetrycapture.Inventory{Timestamp: capturedAt}
			if agent {
				value.Agent = &telemetrycapture.AgentInventoryMetadata{AgentStartupTimeMS: startupMS}
			}
			rebase(&telemetry.Sample{Inventory: value}, start, shift, 1, 1)
			if value.Timestamp != start.UnixNano()+capturedAt+int64(shift) {
				t.Fatal("inventory timestamp lost captured offset, fractional time, or replay cycle shift")
			}
			if agent && value.Agent.AgentStartupTimeMS != start.UnixMilli()+startupMS {
				t.Fatal("repeated inventory changed the Agent startup time")
			}
		}
	}
}
