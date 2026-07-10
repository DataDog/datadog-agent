// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package run

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestServicedefIsEnabledProcmgrFallback(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("process_config.enabled", true)

	svc := Servicedef{
		name: "process",
		configKeys: map[string]model.Reader{
			"process_config.enabled": cfg,
		},
		suppressIf: func() bool { return true },
	}

	assert.False(t, svc.IsEnabled(true), "suppress legacy service when procmgr started")
	assert.True(t, svc.IsEnabled(false), "fall back to legacy service when procmgr unavailable")
}
