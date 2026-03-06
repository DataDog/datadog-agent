// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package securityagentimpl

import (
	"context"
	"expvar"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestGetStatusDetails(t *testing.T) {
	// Publish a test expvar so we can verify it shows up in the response.
	expvar.NewString("test_security_key").Set("test_value")

	impl := &remoteagentImpl{
		cfg: config.NewMock(t),
	}

	resp, err := impl.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp.MainSection)
	assert.Contains(t, resp.MainSection.Fields, "test_security_key")
}

func TestGetFlareFiles_StatusFileMatchesStatusComponent(t *testing.T) {
	statusJSON := []byte(`{"agent":"security-agent","status":"running"}`)
	impl := &remoteagentImpl{
		cfg:        config.NewMock(t),
		statusComp: &statusMock{statusJSON: statusJSON},
	}

	resp, err := impl.GetFlareFiles(context.Background(), &pbcore.GetFlareFilesRequest{})

	require.NoError(t, err)
	assert.Equal(t, statusJSON, resp.Files["security_agent_status.json"])
}
