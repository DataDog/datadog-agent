// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func buildAgentTaskConfig(t *testing.T, taskType, uuid string, args map[string]string) rcclienttypes.AgentTaskConfig {
	t.Helper()
	data := []byte(`{"task_type":"` + taskType + `","uuid":"` + uuid + `","args":{`)
	first := true
	for k, v := range args {
		if !first {
			data = append(data, ',')
		}
		data = append(data, []byte(`"`+k+`":"`+v+`"`)...)
		first = false
	}
	data = append(data, []byte(`}}`)...)
	task, err := rcclienttypes.ParseConfigAgentTask(data, state.Metadata{})
	require.NoError(t, err)
	return task
}

func TestHandleRCFlareTask_MissingCaseID(t *testing.T) {
	cfg := configmock.New(t)
	task := buildAgentTaskConfig(t, "flare", "uuid-1", map[string]string{
		"user_handle": "test@example.com",
	})

	err := HandleRCFlareTask(task, cfg, nil, nil, nil)
	assert.ErrorContains(t, err, "case_id")
}

func TestHandleRCFlareTask_MissingUserHandle(t *testing.T) {
	cfg := configmock.New(t)
	task := buildAgentTaskConfig(t, "flare", "uuid-2", map[string]string{
		"case_id": "12345",
	})

	err := HandleRCFlareTask(task, cfg, nil, nil, nil)
	assert.ErrorContains(t, err, "user_handle")
}

func TestHandleRCFlareTask_CreateArchiveError(t *testing.T) {
	origCreate := createDCAArchiveFunc
	t.Cleanup(func() { createDCAArchiveFunc = origCreate })

	createDCAArchiveFunc = func(_ bool, _, _ string, _ ProfileData, _ status.Component, _ diagnose.Component, _ ipc.Component) (string, error) {
		return "", errors.New("archive creation failed")
	}

	cfg := configmock.New(t)
	task := buildAgentTaskConfig(t, "flare", "uuid-3", map[string]string{
		"case_id":     "12345",
		"user_handle": "test@example.com",
	})

	err := HandleRCFlareTask(task, cfg, nil, nil, nil)
	assert.ErrorContains(t, err, "failed to create cluster-agent flare")
	assert.ErrorContains(t, err, "archive creation failed")
}

func TestHandleRCFlareTask_SendError(t *testing.T) {
	origCreate := createDCAArchiveFunc
	origSend := sendFlareFunc
	t.Cleanup(func() {
		createDCAArchiveFunc = origCreate
		sendFlareFunc = origSend
	})

	createDCAArchiveFunc = func(_ bool, _, _ string, _ ProfileData, _ status.Component, _ diagnose.Component, _ ipc.Component) (string, error) {
		return "/tmp/fake-flare.zip", nil
	}
	sendFlareFunc = func(_ pkgconfigmodel.Reader, _, _, _, _, _ string, _ flarehelpers.FlareSource) (string, error) {
		return "", errors.New("send failed")
	}

	cfg := configmock.New(t)
	task := buildAgentTaskConfig(t, "flare", "uuid-4", map[string]string{
		"case_id":     "12345",
		"user_handle": "test@example.com",
	})

	err := HandleRCFlareTask(task, cfg, nil, nil, nil)
	assert.ErrorContains(t, err, "send failed")
}

func TestHandleRCFlareTask_HappyPath(t *testing.T) {
	origCreate := createDCAArchiveFunc
	origSend := sendFlareFunc
	t.Cleanup(func() {
		createDCAArchiveFunc = origCreate
		sendFlareFunc = origSend
	})

	// Use a real temp file to verify it gets removed after successful upload.
	tmpFile, err := os.CreateTemp("", "flare-*.zip")
	require.NoError(t, err)
	tmpFile.Close()
	tmpPath := tmpFile.Name()

	var capturedCaseID, capturedUserHandle string
	var capturedSource flarehelpers.FlareSource

	createDCAArchiveFunc = func(_ bool, _, _ string, _ ProfileData, _ status.Component, _ diagnose.Component, _ ipc.Component) (string, error) {
		return tmpPath, nil
	}
	sendFlareFunc = func(_ pkgconfigmodel.Reader, _, caseID, userHandle, _, _ string, source flarehelpers.FlareSource) (string, error) {
		capturedCaseID = caseID
		capturedUserHandle = userHandle
		capturedSource = source
		return "flare sent", nil
	}

	cfg := configmock.New(t)
	task := buildAgentTaskConfig(t, "flare", "uuid-5", map[string]string{
		"case_id":     "99999",
		"user_handle": "support@example.com",
	})

	err = HandleRCFlareTask(task, cfg, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "99999", capturedCaseID)
	assert.Equal(t, "support@example.com", capturedUserHandle)
	assert.Equal(t, flarehelpers.NewRemoteConfigFlareSource("uuid-5"), capturedSource)
	_, statErr := os.Stat(tmpPath)
	assert.True(t, os.IsNotExist(statErr), "flare archive should be removed after successful upload")
}

func TestHandleRCFlareTask_NoCleanupOnSendError(t *testing.T) {
	origCreate := createDCAArchiveFunc
	origSend := sendFlareFunc
	t.Cleanup(func() {
		createDCAArchiveFunc = origCreate
		sendFlareFunc = origSend
	})

	tmpFile, err := os.CreateTemp("", "flare-*.zip")
	require.NoError(t, err)
	tmpFile.Close()
	tmpPath := tmpFile.Name()
	t.Cleanup(func() { os.Remove(tmpPath) })

	createDCAArchiveFunc = func(_ bool, _, _ string, _ ProfileData, _ status.Component, _ diagnose.Component, _ ipc.Component) (string, error) {
		return tmpPath, nil
	}
	sendFlareFunc = func(_ pkgconfigmodel.Reader, _, _, _, _, _ string, _ flarehelpers.FlareSource) (string, error) {
		return "", errors.New("send failed")
	}

	cfg := configmock.New(t)
	task := buildAgentTaskConfig(t, "flare", "uuid-6", map[string]string{
		"case_id":     "12345",
		"user_handle": "test@example.com",
	})

	err = HandleRCFlareTask(task, cfg, nil, nil, nil)
	assert.ErrorContains(t, err, "send failed")
	_, statErr := os.Stat(tmpPath)
	assert.NoError(t, statErr, "flare archive should be kept when upload fails")
}
