// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package localapiclientimpl

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func response(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestStatusDecodesResponse(t *testing.T) {
	client := newClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "http://installer/status", req.URL.String())
		require.Equal(t, "application/json", req.Header.Get("Accept"))
		return response(`{"remote_config_state":[{"package":"datadog-agent","stable_version":"7.99.0","task":{"id":"task-1","state":2}}],"secrets_pub_key":"secret"}`), nil
	})}, "installer")

	status, err := client.Status()
	require.NoError(t, err)
	require.Equal(t, "secret", status.SecretsPubKey)
	require.Len(t, status.RemoteConfigState, 1)
	require.Equal(t, "datadog-agent", status.RemoteConfigState[0].Package)
	require.Equal(t, "7.99.0", status.RemoteConfigState[0].StableVersion)
	require.Equal(t, pbgo.TaskState_DONE, status.RemoteConfigState[0].Task.State)
}

func TestStatusReturnsAPIError(t *testing.T) {
	client := newClient(&http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return response(`{"error":{"message":"status unavailable"}}`), nil
	})}, "installer")

	_, err := client.Status()
	require.EqualError(t, err, "error getting status: status unavailable")
}

func TestStatusRejectsMalformedResponse(t *testing.T) {
	client := newClient(&http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return response(`{"remote_config_state":`), nil
	})}, "installer")

	_, err := client.Status()
	require.ErrorContains(t, err, "decoding installer status response")
}

func TestStatusRejectsOversizedResponse(t *testing.T) {
	client := newClient(&http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return response(strings.Repeat(" ", maxStatusResponseSize+1)), nil
	})}, "installer")

	_, err := client.Status()
	require.EqualError(t, err, "installer status response exceeds 1048576 bytes")
}

func TestStatusSetsRequestDeadline(t *testing.T) {
	client := newClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		require.True(t, ok)
		remaining := time.Until(deadline)
		require.Positive(t, remaining)
		require.LessOrEqual(t, remaining, statusRequestTimeout)
		return response(`{}`), nil
	})}, "installer")

	_, err := client.Status()
	require.NoError(t, err)
}

func TestDefaultTransportDisablesKeepAlives(t *testing.T) {
	transport, ok := newHTTPClient().Transport.(*http.Transport)
	require.True(t, ok)
	require.True(t, transport.DisableKeepAlives)
}
