// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package tracer

import (
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
)

func platformInit() {
	_ = driver.Init(&sysconfigtypes.Config{})
}

func httpSupported() bool {
	return false
}

//nolint:revive // TODO(WKIT) Fix revive linter
func classificationSupported(config *config.Config) bool {
	return true
}

func testConfig() *config.Config {
	cfg := config.New()
	return cfg
}
