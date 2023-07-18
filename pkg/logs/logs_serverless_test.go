// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package logs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildServerlessEndpoints(t *testing.T) {
	endpoints, err := buildEndpoints()
	assert.Nil(t, err)
	assert.Equal(t, "http-intake.logs.datadoghq.com", endpoints.Main.Host)
	assert.Equal(t, "lambda-extension", string(endpoints.Main.Origin))
}
