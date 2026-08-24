// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_datadogagent

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestGetDogstatsdTopHandler(t *testing.T) {
	client := &fakeIPCClient{
		post: func(endpointURL, contentType string, body io.Reader, _ ...ipc.RequestOption) ([]byte, error) {
			require.True(t, strings.HasSuffix(endpointURL, "/agent/dogstatsd-contexts-top"))
			require.Equal(t, "application/json", contentType)
			payload, err := io.ReadAll(body)
			require.NoError(t, err)
			require.JSONEq(t, `{"num_metrics":20,"num_tags":10}`, string(payload))
			return []byte(`{"metrics":[]}`), nil
		},
	}
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]interface{}{"num_metrics": 20, "num_tags": 10},
	}

	result, err := NewGetDogstatsdTopHandler(client).Run(context.Background(), task, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"metrics": []interface{}{}}, result)
}
