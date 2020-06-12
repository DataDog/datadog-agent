// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package http

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var testHTTPClientFactory = func() *http.Client {
	return &http.Client{}
}

func TestResetClient_ShouldReset(t *testing.T) {
	client := NewResetClient(
		1*time.Nanosecond,
		testHTTPClientFactory,
	)

	initialHTTPClient := client.httpClient
	time.Sleep(1 * time.Millisecond)
	client.checkReset()

	assert.NotSame(t, initialHTTPClient, client.httpClient)
}

func TestResetClient_ZeroIntervalShouldNotReset(t *testing.T) {
	client := NewResetClient(
		0,
		testHTTPClientFactory,
	)

	initialHTTPClient := client.httpClient
	time.Sleep(1 * time.Millisecond)
	client.checkReset()

	assert.Same(t, initialHTTPClient, client.httpClient)
}
