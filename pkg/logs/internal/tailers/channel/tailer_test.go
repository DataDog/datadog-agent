// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestComputeServiceName(t *testing.T) {
	assert.Equal(t, "agent", computeServiceName(nil, "toto"))
	lambdaConfig := &config.Lambda{}
	assert.Equal(t, "my-service-name", computeServiceName(lambdaConfig, "my-service-name"))
	assert.Equal(t, "my-service-name", computeServiceName(lambdaConfig, "MY-SERVICE-NAME"))
	assert.Equal(t, "", computeServiceName(lambdaConfig, ""))
}

func TestComputeServiceNameFromCloudRunRevision(t *testing.T) {
	os.Setenv("K_REVISION", "version-abc")
	defer os.Unsetenv("K_REVISION")
	os.Setenv("K_SERVICE", "superService")
	assert.Equal(t, "service-value", computeServiceName(nil, "service-value"))
	assert.Equal(t, "superservice", computeServiceName(nil, ""))
}

func TestNotServerlessModeKVersionUndefined(t *testing.T) {
	os.Setenv("K_SERVICE", "superService")
	defer os.Unsetenv("K_SERVICE")
	assert.False(t, isServerlessOrigin(nil))
}

func TestNotServerlessModeKServiceUndefined(t *testing.T) {
	os.Setenv("K_REVISION", "version-abc")
	defer os.Unsetenv("K_REVISION")
	assert.False(t, isServerlessOrigin(nil))
}

func TestServerlessModeCloudRun(t *testing.T) {
	os.Setenv("K_REVISION", "version-abc")
	defer os.Unsetenv("K_REVISION")
	os.Setenv("K_SERVICE", "superService")
	defer os.Unsetenv("K_SERVICE")
	assert.True(t, isServerlessOrigin(nil))
}

func TestServerlessModeLambda(t *testing.T) {
	lambdaConfig := &config.Lambda{}
	assert.True(t, isServerlessOrigin(lambdaConfig))
}

func TestBuildMessageNoLambda(t *testing.T) {
	logline := &config.ChannelMessage{
		Content:   []byte("bababang"),
		Timestamp: time.Date(2010, 01, 01, 01, 01, 01, 00, time.UTC),
		IsError:   false,
	}
	origin := &message.Origin{}
	builtMessage := buildMessage(logline, origin)
	assert.Equal(t, "bababang", string(builtMessage.Content))
	assert.Nil(t, builtMessage.Lambda)
	assert.Equal(t, message.StatusInfo, builtMessage.GetStatus())
}

func TestBuildMessageLambda(t *testing.T) {
	logline := &config.ChannelMessage{
		Content:   []byte("bababang"),
		Timestamp: time.Date(2010, 01, 01, 01, 01, 01, 00, time.UTC),
		IsError:   false,
		Lambda: &config.Lambda{
			ARN:       "myTestARN",
			RequestID: "myTestRequestId",
		},
	}
	origin := &message.Origin{}
	builtMessage := buildMessage(logline, origin)
	assert.Equal(t, "bababang", string(builtMessage.Content))
	assert.Equal(t, "myTestARN", builtMessage.Lambda.ARN)
	assert.Equal(t, "myTestRequestId", builtMessage.Lambda.RequestID)
	assert.Equal(t, message.StatusInfo, builtMessage.GetStatus())
}

func TestBuildErrorMessage(t *testing.T) {
	logline := &config.ChannelMessage{
		Content:   []byte("bababang"),
		Timestamp: time.Date(2010, 01, 01, 01, 01, 01, 00, time.UTC),
		IsError:   true,
	}
	origin := &message.Origin{}
	builtMessage := buildMessage(logline, origin)
	assert.Equal(t, "bababang", string(builtMessage.Content))
	assert.Equal(t, message.StatusError, builtMessage.GetStatus())
}
