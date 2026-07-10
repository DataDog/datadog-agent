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

func TestServicedefIsEnabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("process_config.enabled", true, model.SourceDefault)

	svc := Servicedef{
		name: "process",
		configKeys: map[string]model.Reader{
			"process_config.enabled": cfg,
		},
	}

	t.Run("enabled by config", func(t *testing.T) {
		assert.True(t, svc.IsEnabled(false, cfg))
		assert.True(t, svc.IsEnabled(true, cfg))
	})

	t.Run("not suppressed without definition file", func(t *testing.T) {
		cfg.Set("process_manager.enabled", true, model.SourceDefault)
		assert.True(t, svc.IsEnabled(true, cfg))
	})

	t.Run("not suppressed when process manager disabled", func(t *testing.T) {
		svc.procmgrDefinitionFile = "datadog-agent-process.yaml"
		cfg.Set("process_manager.enabled", false, model.SourceDefault)
		assert.True(t, svc.IsEnabled(true, cfg))
	})
}
