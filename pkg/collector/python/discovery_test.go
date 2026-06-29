// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"
)

func TestDiscoverConfig(t *testing.T) {
	testDiscoverConfig(t)
}

func TestDiscoverConfigNoConfigs(t *testing.T) {
	testDiscoverConfigNoConfigs(t)
}

func TestDiscoverConfigCustomCheck(t *testing.T) {
	testDiscoverConfigCustomCheck(t)
}

func TestDiscoverConfigRtloaderError(t *testing.T) {
	testDiscoverConfigRtloaderError(t)
}

func TestDiscoverConfigReturnsMalformedResult(t *testing.T) {
	testDiscoverConfigReturnsMalformedResult(t)
}
