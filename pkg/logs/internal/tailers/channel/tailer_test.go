// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/stretchr/testify/assert"
)

func TestComputeServiceName(t *testing.T) {
	assert.Equal(t, "agent", computeServiceName(nil, "toto"))
	lambdaConfig := &config.Lambda{}
	assert.Equal(t, "my-service-name", computeServiceName(lambdaConfig, "my-service-name"))
	assert.Equal(t, "my-service-name", computeServiceName(lambdaConfig, "MY-SERVICE-NAME"))
	assert.Equal(t, "", computeServiceName(lambdaConfig, ""))
}
