// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_datadogagent

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type fakeIPCClient struct {
	post func(string, string, io.Reader, ...ipc.RequestOption) ([]byte, error)
}

func (c *fakeIPCClient) Do(*http.Request, ...ipc.RequestOption) ([]byte, error) {
	panic("unexpected call")
}

func (c *fakeIPCClient) Get(string, ...ipc.RequestOption) ([]byte, error) {
	panic("unexpected call")
}

func (c *fakeIPCClient) Head(string, ...ipc.RequestOption) ([]byte, error) {
	panic("unexpected call")
}

func (c *fakeIPCClient) Post(endpointURL, contentType string, body io.Reader, options ...ipc.RequestOption) ([]byte, error) {
	return c.post(endpointURL, contentType, body, options...)
}

func (c *fakeIPCClient) PostChunk(string, string, io.Reader, func([]byte), ...ipc.RequestOption) error {
	panic("unexpected call")
}

func (c *fakeIPCClient) PostForm(string, url.Values, ...ipc.RequestOption) ([]byte, error) {
	panic("unexpected call")
}

func (c *fakeIPCClient) NewIPCEndpoint(string) (ipc.Endpoint, error) {
	panic("unexpected call")
}

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
