// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package registration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

type ClientMock struct {
}

func (c *ClientMock) Do(*http.Request) (*http.Response, error) {
	return &http.Response{}, nil
}

func TestBuildUrlPrefixEmpty(t *testing.T) {
	builtURL := BuildURL("/myPath")
	assert.Equal(t, "http://localhost:9001/myPath", builtURL)
}

func TestBuildUrlWithPrefix(t *testing.T) {
	t.Setenv("AWS_LAMBDA_RUNTIME_API", "myPrefix:3000")
	builtURL := BuildURL("/myPath")
	assert.Equal(t, "http://myPrefix:3000/myPath", builtURL)
}
