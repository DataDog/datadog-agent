// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/process/encoding"
	reqEncoding "github.com/DataDog/datadog-agent/pkg/process/encoding/request"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	processProto "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

func TestStatsHandler(t *testing.T) {
	mockProbe := mocks.NewProbe(t)
	mockProbe.On("StatsWithPermByPID", []int32{1}).Return(map[int32]*procutil.StatsWithPerm{
		1: {
			OpenFdCount: 10,
			IOStat: &procutil.IOCountersStat{
				ReadCount:  1,
				WriteCount: 1,
				ReadBytes:  1,
				WriteBytes: 1,
			},
		},
	}, nil)

	m := process{
		probe: mockProbe,
	}

	rec := httptest.NewRecorder()

	reqProto := &processProto.ProcessStatRequest{
		Pids: []int32{1},
	}
	reqBody, err := reqEncoding.GetMarshaler(encoding.ContentTypeProtobuf).Marshal(reqProto)
	require.NoError(t, err)

	req, err := http.NewRequest("GET", "/stats", bytes.NewReader(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", encoding.ContentTypeProtobuf)

	m.statsHandler(rec, req)

	resp := rec.Result()
	resBody := resp.Body
	defer resBody.Close()

	resBytes, err := io.ReadAll(resBody)
	require.NoError(t, err)

	contentType := resp.Header.Get("Content-type")
	results, err := encoding.GetUnmarshaler(contentType).Unmarshal(resBytes)
	require.NoError(t, err)

	expectedResponse := &model.ProcStatsWithPermByPID{
		StatsByPID: map[int32]*model.ProcStatsWithPerm{
			1: {
				OpenFDCount: 10,
				ReadCount:   1,
				WriteCount:  1,
				ReadBytes:   1,
				WriteBytes:  1,
			},
		},
	}

	assert.True(t, reflect.DeepEqual(expectedResponse, results))
}
