// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package http

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestBuildURLShouldReturnHTTPSWithUseSSL(t *testing.T) {
	url := builURL(config.Endpoint{
		APIKey: "bar",
		Host:   "foo",
		UseSSL: true,
	})
	assert.Equal(t, "https://foo/v1/input/bar", url)
}

func TestBuildURLShouldReturnHTTPWithoutUseSSL(t *testing.T) {
	url := builURL(config.Endpoint{
		APIKey: "bar",
		Host:   "foo",
		UseSSL: false,
	})
	assert.Equal(t, "http://foo/v1/input/bar", url)
}

func TestBuildURLShouldReturnAddressWithPortWhenDefined(t *testing.T) {
	url := builURL(config.Endpoint{
		APIKey: "bar",
		Host:   "foo",
		Port:   1234,
		UseSSL: false,
	})
	assert.Equal(t, "http://foo:1234/v1/input/bar", url)
}
