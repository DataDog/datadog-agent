// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package serverimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
)

func TestGetDefaultMetricSourceMicroVM(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, "arn:aws:lambda:us-east-1:123456789012:microvm-image:my-image")
	assert.Equal(t, metrics.MetricSourceAWSMicroVMCustom, GetDefaultMetricSource())
}

func TestGetDefaultMetricSourceFallback(t *testing.T) {
	assert.Equal(t, metrics.MetricSourceServerless, GetDefaultMetricSource())
}
