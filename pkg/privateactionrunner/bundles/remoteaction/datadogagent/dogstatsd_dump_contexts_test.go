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

func TestDumpDogstatsdContextsHandler(t *testing.T) {
	client := &fakeIPCClient{
		post: func(endpointURL, contentType string, body io.Reader, _ ...ipc.RequestOption) ([]byte, error) {
			require.True(t, strings.HasSuffix(endpointURL, "/agent/dogstatsd-contexts-dump"))
			require.Equal(t, "application/json", contentType)
			require.Nil(t, body)
			return []byte(`"/opt/datadog-agent/run/dogstatsd_contexts.json.zstd"`), nil
		},
	}

	result, err := NewDumpDogstatsdContextsHandler(client).Run(context.Background(), &types.Task{}, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{
		"path": "/opt/datadog-agent/run/dogstatsd_contexts.json.zstd",
	}, result)
}
