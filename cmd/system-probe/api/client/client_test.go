// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstructURL(t *testing.T) {
	u := constructURL("", "/asdf?a=b")
	assert.Equal(t, "http://sysprobe/asdf?a=b", u)

	u = constructURL("zzzz", "/asdf?a=b")
	assert.Equal(t, "http://sysprobe/zzzz/asdf?a=b", u)

	u = constructURL("zzzz", "asdf")
	assert.Equal(t, "http://sysprobe/zzzz/asdf", u)
}

func TestGetCheck(t *testing.T) {
	type testData struct {
		Str string
		Num int
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test/check" {
			_, _ = w.Write([]byte(`{"Str": "asdf", "Num": 42}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client := &http.Client{Transport: &http.Transport{DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("tcp", server.Listener.Addr().String())
	}}}

	resp, err := GetCheck[testData](client, "test")
	require.NoError(t, err)
	assert.Equal(t, "asdf", resp.Str)
	assert.Equal(t, 42, resp.Num)
}
