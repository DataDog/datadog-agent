// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the sysprobeconfig component.
package mock

import (
	"os"
	"strings"
	"testing"

	sysprobeconfigdef "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeconfigimpl "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

// NewMock creates a mock for the sysprobeconfig component.
func NewMock(t testing.TB) sysprobeconfigdef.Component {
	return NewMockWithOverrides(t, nil)
}

// NewMockWithOverrides creates a mock for the sysprobeconfig component with
// config overrides applied before sysconfig.New() is called, so that
// SysProbeObject() reflects them.
func NewMockWithOverrides(t testing.TB, overrides map[string]interface{}) sysprobeconfigdef.Component {
	cfg := configmock.NewSystemProbe(t)
	for k, v := range overrides {
		cfg.SetInTest(k, v)
	}

	// The config automatically load environment variables starting by DD_*,
	// so we must also strip all `DD_` environment variables for the duration of the test.
	oldEnv := os.Environ()
	for _, kv := range oldEnv {
		if strings.HasPrefix(kv, "DD_") {
			kvslice := strings.SplitN(kv, "=", 2)
			_ = os.Unsetenv(kvslice[0])
		}
	}
	t.Cleanup(func() {
		for _, kv := range oldEnv {
			kvslice := strings.SplitN(kv, "=", 2)
			os.Setenv(kvslice[0], kvslice[1])
		}
	})

	syscfg, err := sysconfig.New("", "")
	if err != nil {
		t.Fatalf("sysprobe config create: %s", err)
	}
	return sysprobeconfigimpl.NewTestComponent(pkgconfigsetup.SystemProbe(), syscfg)
}
