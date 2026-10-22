// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestCreatorCreateWithArgs(t *testing.T) {
	origCreate := createDCAArchiveFunc
	origIdentity := getClusterAgentIdentityFunc
	t.Cleanup(func() {
		createDCAArchiveFunc = origCreate
		getClusterAgentIdentityFunc = origIdentity
	})

	tmpPath := filepath.Join(t.TempDir(), "datadog-agent-2026-10-19T15-00-00Z-info.zip")
	require.NoError(t, os.WriteFile(tmpPath, nil, 0600))

	var capturedLocal bool
	var capturedArgs flaretypes.FlareArgs
	createDCAArchiveFunc = func(local bool, _, _ string, _ ProfileData, flareArgs flaretypes.FlareArgs, _ status.Component, _ diagnose.Component, _ ipc.Component) (string, error) {
		capturedLocal = local
		capturedArgs = flareArgs
		return tmpPath, nil
	}
	getClusterAgentIdentityFunc = func() (string, string, string, error) {
		return "my-cluster", "datadog", "datadog-cluster-agent-abc123", nil
	}

	flareArgs := flaretypes.FlareArgs{
		ProfileDuration:      30 * time.Second,
		ProfileBlockingRate:  10000,
		ProfileMutexFraction: 100,
	}
	path, err := NewCreator(configmock.New(t), nil, nil, nil).CreateWithArgs(flareArgs, 0, nil, []byte{})
	require.NoError(t, err)

	assert.False(t, capturedLocal)
	assert.Equal(t, flareArgs, capturedArgs)
	assert.Equal(t, filepath.Join(filepath.Dir(tmpPath), "datadog-agent-my-cluster__datadog__datadog-cluster-agent-abc123__2026-10-19T15-00-00Z-info.zip"), path)
	assert.FileExists(t, path)
}

func TestCreatorSend(t *testing.T) {
	origSend := sendFlareFunc
	t.Cleanup(func() { sendFlareFunc = origSend })

	var capturedPath, capturedCaseID, capturedEmail, capturedAPIKey string
	var capturedSource flarehelpers.FlareSource
	sendFlareFunc = func(_ pkgconfigmodel.Reader, archivePath, caseID, email, apiKey, _ string, source flarehelpers.FlareSource) (string, error) {
		capturedPath = archivePath
		capturedCaseID = caseID
		capturedEmail = email
		capturedAPIKey = apiKey
		capturedSource = source
		return "flare sent", nil
	}

	cfg := configmock.New(t)
	cfg.Set("api_key", "test-api-key", pkgconfigmodel.SourceAgentRuntime)

	response, err := NewCreator(cfg, nil, nil, nil).Send("/tmp/flare.zip", "12345", "test@example.com", flarehelpers.NewLocalFlareSource())
	require.NoError(t, err)

	assert.Equal(t, "flare sent", response)
	assert.Equal(t, "/tmp/flare.zip", capturedPath)
	assert.Equal(t, "12345", capturedCaseID)
	assert.Equal(t, "test@example.com", capturedEmail)
	assert.Equal(t, "test-api-key", capturedAPIKey)
	assert.Equal(t, flarehelpers.NewLocalFlareSource(), capturedSource)
}
