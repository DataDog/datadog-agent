// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package tracer

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"

	"testing"
)

func httpSupported(t *testing.T) bool {
	return false
}

func classificationSupported(config *config.Config) bool {
	return true
}

func testConfig() *config.Config {
	cfg := config.New()
	return cfg
}
