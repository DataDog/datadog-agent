// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package traceimpl

import (
	"context"
	"encoding/json"
	"expvar"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestGetStatusDetails(t *testing.T) {
	// Publish targeted expvars that the trace agent status template needs.
	expvar.NewString("pid").Set("99999")
	expvar.Publish("uptime_test_trace", expvar.Func(func() interface{} { return 42 }))

	impl := &remoteagentImpl{
		cfg: config.NewMock(t),
	}

	resp, err := impl.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp.MainSection)
	require.Contains(t, resp.MainSection.Fields, "status")

	// Verify the "status" field contains valid JSON with targeted trace agent keys.
	var st map[string]interface{}
	err = json.Unmarshal([]byte(resp.MainSection.Fields["status"]), &st)
	require.NoError(t, err)

	// Should contain targeted trace agent fields.
	assert.Contains(t, st, "pid")

	// Should NOT contain unrelated expvars.
	assert.NotContains(t, st, "uptime_test_trace")
	assert.NotContains(t, st, "test_security_key")
}
