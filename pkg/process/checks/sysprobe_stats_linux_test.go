// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package checks

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	processpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

type recordingProcessStatsTransport struct {
	requests [][]int32
	err      error
}

func (t *recordingProcessStatsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.err = err
		return nil, err
	}

	request := &processpb.ProcessStatRequest{}
	if err := proto.Unmarshal(body, request); err != nil {
		t.err = err
		return nil, err
	}
	t.requests = append(t.requests, request.Pids)

	return nil, errors.New("request recorded")
}

func newRecordingProcessStatsClient() (*http.Client, *recordingProcessStatsTransport) {
	transport := &recordingProcessStatsTransport{}
	return &http.Client{Transport: transport}, transport
}

func TestFetchSystemProbeStatsForProcesses(t *testing.T) {
	t.Run("mixed processes only request live pid", func(t *testing.T) {
		client, transport := newRecordingProcessStatsClient()
		running := makeProcess(1, "running")
		zombie := makeProcess(2, "zombie")
		zombie.Stats.Status = "Z"

		_, err := fetchSystemProbeStatsForProcesses(client, map[int32]*procutil.Process{1: running, 2: zombie})
		require.Error(t, err)
		require.NoError(t, transport.err)
		require.Len(t, transport.requests, 1)
		assert.Equal(t, []int32{1}, transport.requests[0])
	})

	t.Run("all zombies skip request", func(t *testing.T) {
		client, transport := newRecordingProcessStatsClient()
		zombie := makeProcess(2, "zombie")
		zombie.Stats.Status = "Z"

		stats, err := fetchSystemProbeStatsForProcesses(client, map[int32]*procutil.Process{2: zombie})
		require.NoError(t, err)
		assert.Nil(t, stats)
		assert.Empty(t, transport.requests)
	})
}

func TestMergeStatWithSysprobeStatsRequest(t *testing.T) {
	t.Run("mixed stats only request live pid", func(t *testing.T) {
		client, transport := newRecordingProcessStatsClient()
		stats := map[int32]*procutil.Stats{
			1: {Status: "R", IOStat: &procutil.IOCountersStat{}},
			2: {Status: "Z", IOStat: &procutil.IOCountersStat{}},
		}

		mergeStatWithSysprobeStats([]int32{1, 2}, stats, client)
		require.NoError(t, transport.err)
		require.Len(t, transport.requests, 1)
		assert.Equal(t, []int32{1}, transport.requests[0])
	})

	t.Run("all zombies skip request", func(t *testing.T) {
		client, transport := newRecordingProcessStatsClient()
		stats := map[int32]*procutil.Stats{
			2: {Status: "Z", IOStat: &procutil.IOCountersStat{}},
		}

		mergeStatWithSysprobeStats([]int32{2}, stats, client)
		assert.Empty(t, transport.requests)
	})
}
