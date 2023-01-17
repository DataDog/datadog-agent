// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestBuildStoreKey(t *testing.T) {
	res := buildStoreKey()
	assert.Equal(t, "/datadog/check_configs", res)
	res = buildStoreKey("")
	assert.Equal(t, "/datadog/check_configs", res)
	res = buildStoreKey("foo")
	assert.Equal(t, "/datadog/check_configs/foo", res)
	res = buildStoreKey("foo", "bar")
	assert.Equal(t, "/datadog/check_configs/foo/bar", res)
	res = buildStoreKey("foo", "bar", "bazz")
	assert.Equal(t, "/datadog/check_configs/foo/bar/bazz", res)
}

func TestGetPollInterval(t *testing.T) {
	cp := config.ConfigurationProviders{}
	assert.Equal(t, GetPollInterval(cp), 10*time.Second)
	cp = config.ConfigurationProviders{
		PollInterval: "foo",
	}
	assert.Equal(t, GetPollInterval(cp), 10*time.Second)
	cp = config.ConfigurationProviders{
		PollInterval: "1s",
	}
	assert.Equal(t, GetPollInterval(cp), 1*time.Second)
}
