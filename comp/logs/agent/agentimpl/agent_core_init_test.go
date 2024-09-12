// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
)

func TestBuildEndpoints(t *testing.T) {
	config := config.NewMock(t)

	endpoints, err := buildEndpoints(config)
	assert.Nil(t, err)
	assert.Equal(t, "agent-intake.logs.datadoghq.com", endpoints.Main.Host)
}
