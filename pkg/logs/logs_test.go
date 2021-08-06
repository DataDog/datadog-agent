// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildServerlessEndpoints(t *testing.T) {
	endpoints, err := buildEndpoints(true)
	assert.Nil(t, err)
	assert.Equal(t, "lambda-http-intake.logs.datadoghq.com", endpoints.Main.Host)
}

func TestBuildEndpoints(t *testing.T) {
	endpoints, err := buildEndpoints(false)
	assert.Nil(t, err)
	assert.Equal(t, "agent-intake.logs.datadoghq.com", endpoints.Main.Host)
}
