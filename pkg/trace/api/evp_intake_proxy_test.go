// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"testing"

	"github.com/stretchr/testify/require"
)

type roundTripperMock func(*http.Request) (*http.Response, error)

func (r roundTripperMock) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

func TestEvpIntakeReverseProxyHandler(t *testing.T) {

	conf := newTestReceiverConfig()
	conf.Site = "my.site.com"
	conf.Endpoints[0].APIKey = "test_api_key"

	request := httptest.NewRequest("POST", "/mysubdomain/mypath/mysubpath", nil)

	responseBody := []byte("{\"potato\": true}")
	mockRoundTripper := roundTripperMock(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, req.Host, "mysubdomain.my.site.com")
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBuffer(responseBody)),
		}, nil
	})

	handler := evpIntakeReverseProxyHandler(conf, evpIntakeEndpointsFromConfig(conf), mockRoundTripper)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, request)
	out, _ := ioutil.ReadAll(rec.Body)
	require.Equal(t, http.StatusOK, rec.Code, "Got: ", fmt.Sprint(rec.Code))
	require.Equal(t, responseBody, out, "Got: ", string(out))
}
