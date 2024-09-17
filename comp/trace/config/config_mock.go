// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package config

import (
	"testing"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/env"
)

// newMock exported mock builder to allow modifying mocks that might be
// supplied in tests and used for dep injection.
func newMock(deps Dependencies, _ testing.TB) (Component, error) {
	traceCfg, err := setupConfig(deps, "apikey")
	if err != nil {
		return nil, err
	}

	c := cfg{
		warnings:    &pkgconfig.Warnings{},
		coreConfig:  deps.Config,
		AgentConfig: traceCfg,
	}

	c.SetMaxMemCPU(env.IsContainerized())

	return &c, nil
}
