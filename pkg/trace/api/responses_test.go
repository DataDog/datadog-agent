// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-go/v5/statsd"
)

func TestWriteCounter(t *testing.T) {
	assert := assert.New(t)
	var buf bytes.Buffer
	wc := newWriteCounter(&buf)
	assert.Zero(wc.N())
	wc.Write([]byte{1})
	wc.Write([]byte{2})
	assert.EqualValues(wc.N(), 2)
	assert.EqualValues(buf.Bytes(), []byte{1, 2})
	wc.Write([]byte{3})
	assert.EqualValues(wc.N(), 3)
	assert.EqualValues(buf.Bytes(), []byte{1, 2, 3})
}

type testResponseWriter struct {
	response   string
	statusCode int
	header     http.Header
}

func (w *testResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}

	return w.header
}

func (w *testResponseWriter) Write(b []byte) (int, error) {
	w.response += string(b)
	return len(b), nil
}

func (w *testResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func TestHTTPRateByService(t *testing.T) {
	assert := assert.New(t)
	dc := sampler.NewDynamicConfig()
	for i, tt := range []struct {
		version  string
		response string
		header   http.Header
	}{
		{
			version:  "",
			response: "{\"rate_by_service\":{}}\n",
			header: http.Header{
				"Content-Type": []string{"application/json"},
			},
		},
		{
			version:  "-",
			response: "{\"rate_by_service\":{}}\n",
			header: http.Header{
				"Content-Type":                  []string{"application/json"},
				"Datadog-Rates-Payload-Version": []string{""},
			},
		},
	} {
		rw := testResponseWriter{}
		httpRateByService(tt.version, &rw, dc, &statsd.NoOpClient{})
		assert.Equal(tt.header, rw.Header(), strconv.Itoa(i))
		assert.Equal(tt.response, rw.response, strconv.Itoa(i))
	}
}
