// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/internal/anomalydetection/scorer/scoring"
)

func TestHealthyRecordingMetadata(t *testing.T) {
	data := []byte(`{"name":"healthy-recording","scenario":"my-app","mode":"healthy-control","expectation":"healthy","disruptions":[],"evaluation_window":{"start":"2026-01-01T00:05:00Z","end":"2026-01-01T00:35:00Z"}}`)
	var info EpisodeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	if info.Scenario.AppName != "my-app" || info.Mode != "healthy-control" || info.Disruption != nil || info.Cooldown != nil {
		t.Fatalf("lost healthy metadata: %+v", info)
	}
	if _, _, err := scoring.HealthyWindow(info.HealthyEpisode); err != nil {
		t.Fatal(err)
	}
	// The explicit disruption must remain visible to validation even though the
	// testbench also has a typed phase field with the same JSON name.
	bad := []byte(`{"mode":"healthy-control","expectation":"healthy","disruption":{"start":"2026-01-01T00:10:00Z"}}`)
	if err := json.Unmarshal(bad, &info); err != nil {
		t.Fatal(err)
	}
	if _, _, err := scoring.HealthyWindow(info.HealthyEpisode); err == nil {
		t.Fatal("accepted healthy recording containing a fault")
	}
}
