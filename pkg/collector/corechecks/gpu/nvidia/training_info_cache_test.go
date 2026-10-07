// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server/testutil"
)

func TestTrainingInfoCacheRefresh(t *testing.T) {
	socketPath := testutil.SystemProbeSocketPath(t, "training-info-cache")
	server, err := testutil.NewSystemProbeTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/stats":
			_, _ = w.Write([]byte(`{}`))
		case "/gpu/training-info":
			_, _ = w.Write([]byte(`[
				{"pid":10,"device_uuid":"GPU-1","training_run_id":"run-a","training_group_id":"group"},
				{"pid":20,"device_uuid":"GPU-1","training_run_id":"run-b","training_group_id":"group"}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}), socketPath)
	require.NoError(t, err)
	server.Start()
	t.Cleanup(server.Close)

	cache := NewTrainingInfoCache(sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath)))

	require.NoError(t, cache.Refresh())
	assert.Equal(t, []string{"training_group_id:group", "training_run_id:run-a", "training_run_id:run-b"}, cache.DeviceTags("GPU-1"))
	assert.Empty(t, cache.DeviceTags("GPU-2"))
	assert.Equal(t, []string{"training_group_id:group", "training_run_id:run-a"}, cache.ProcessTags(10))
	assert.Equal(t, []string{"training_group_id:group", "training_run_id:run-a", "training_run_id:run-b"}, cache.ProcessTags(10, 20))
	assert.Empty(t, cache.ProcessTags(30))
	assert.Empty(t, cache.ProcessTags())
}

func TestTrainingInfoTags(t *testing.T) {
	deviceTags, processTags := trainingInfoTags([]model.TrainingInfo{
		{PID: 1, DeviceUUID: "GPU-1", TrainingRunID: "run-a", TrainingGroupID: "group"},
		{PID: 2, DeviceUUID: "GPU-1", TrainingRunID: "run-a", TrainingGroupID: "group"},
		{PID: 3, DeviceUUID: "GPU-1", TrainingRunID: "run-b"},
		{PID: 3, DeviceUUID: "GPU-2", TrainingGroupID: "group"},
	})

	assert.Equal(t, map[string][]string{
		"GPU-1": {"training_group_id:group", "training_run_id:run-a", "training_run_id:run-b"},
		"GPU-2": {"training_group_id:group"},
	}, deviceTags)
	assert.Equal(t, map[uint32][]string{
		1: {"training_group_id:group", "training_run_id:run-a"},
		2: {"training_group_id:group", "training_run_id:run-a"},
		3: {"training_group_id:group", "training_run_id:run-b"},
	}, processTags)
}
