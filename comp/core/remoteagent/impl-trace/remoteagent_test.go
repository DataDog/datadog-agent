// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package traceimpl

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestGetStatusDetails(t *testing.T) {
	impl := &remoteagentImpl{
		cfg: config.NewMock(t),
	}

	resp, err := impl.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp.MainSection)
	require.Contains(t, resp.MainSection.Fields, "status")

	// Verify the "status" field is valid JSON matching StatusInfo's structure.
	var st map[string]interface{}
	err = json.Unmarshal([]byte(resp.MainSection.Fields["status"]), &st)
	require.NoError(t, err)

	// pid is read directly from os.Getpid(), not from expvar.
	assert.Equal(t, strconv.Itoa(os.Getpid()), st["pid"])

	// All StatusInfo fields are present (zero/nil values before InitInfo).
	assert.Contains(t, st, "uptime")
	assert.Contains(t, st, "receiver")
	assert.Contains(t, st, "ratebyservice_filtered")
	assert.Contains(t, st, "trace_writer")
	assert.Contains(t, st, "stats_writer")
	assert.Contains(t, st, "watchdog")
	assert.Contains(t, st, "memstats")
	assert.Contains(t, st, "version")
}
