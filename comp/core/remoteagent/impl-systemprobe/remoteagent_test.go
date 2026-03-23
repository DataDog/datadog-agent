// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package systemprobeimpl

import (
	"context"
	"encoding/json"
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

	// Verify the "status" field contains valid JSON from module.GetStats().
	var stats map[string]interface{}
	err = json.Unmarshal([]byte(resp.MainSection.Fields["status"]), &stats)
	require.NoError(t, err)

	// module.GetStats() returns at minimum the global fields when no modules are loaded.
	// The key point is that it's valid JSON and not a dump of all expvars.
	assert.IsType(t, map[string]interface{}{}, stats)
}
