// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_datadogagent

import (
	"io"
	"net/http"
	"net/url"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
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
