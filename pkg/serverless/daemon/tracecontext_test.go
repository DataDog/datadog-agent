// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTraceContextEndpoint(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon("127.0.0.1:8124")
	defer d.Stop()
	client := &http.Client{Timeout: 1 * time.Second}
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8124/lambda/trace-context", nil)
	assert.Nil(err)
	response, err := client.Do(request)
	assert.Nil(err)
	assert.NotEmpty(response.Header.Get("x-datadog-trace-id"))
	assert.NotEmpty(response.Header.Get("x-datadog-span-id"))
}
