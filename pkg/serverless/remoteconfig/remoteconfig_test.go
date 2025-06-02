// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
)

func TestRemoteConfigDisabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("remote_configuration.enabled", false)
	rcService := StartRCService("arn:aws:lambda:sa-east-1:123456789123:function:my-test-func")
	assert.Nil(t, rcService)
}

func TestRemoteConfigEnabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("remote_configuration.enabled", true)
	rcService := StartRCService("arn:aws:lambda:sa-east-1:123456789123:function:my-test-func")
	assert.NotNil(t, rcService)
}
