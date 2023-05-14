// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestComputeServiceNameOrderOfPrecedent(t *testing.T) {
	assert.Equal(t, "agent", getServiceName())
	t.Setenv("CONTAINER_APP_NAME", "CONTAINER-app-name")
	assert.Equal(t, "container-app-name", getServiceName())
	t.Setenv("K_SERVICE", "CLOUD-run-service-name")
	assert.Equal(t, "cloud-run-service-name", getServiceName())
	t.Setenv("DD_SERVICE", "DD-service-name")
	assert.Equal(t, "dd-service-name", getServiceName())
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
