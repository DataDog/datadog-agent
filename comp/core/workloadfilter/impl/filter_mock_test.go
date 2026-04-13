// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package workloadfilterimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/mock"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestNewMock_ProvidesMockFilter(t *testing.T) {
	// Use mock components for dependencies
	cfg := config.NewMock(t)
	logger := logmock.New(t)
	telemetry := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())

	req := MockRequires{
		Config:    cfg,
		Log:       logger,
		Telemetry: telemetry,
	}

	provides := NewMock(req)
	assert.NotNil(t, provides.Comp, "Mock filter component should not be nil")

	// Check that the returned component implements the expected interface
	_, ok := interface{}(provides.Comp).(mock.Mock)
	assert.True(t, ok, "Returned component should implement workloadfiltermock.Mock")
}

func TestNewMock_UsesMockConfig(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("container_exclude", "name:excluded-container")

	logger := logmock.New(t)
	telemetry := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())

	req := MockRequires{
		Config:    cfg,
		Log:       logger,
		Telemetry: telemetry,
	}
	provides := NewMock(req)

	// Does not exclude by default
	container := workloadmetafilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "included-container",
			},
		},
		nil,
	)
	filterBundle := provides.Comp.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.ContainerLegacyGlobal}})
	res := filterBundle.IsExcluded(container)
	assert.Equal(t, false, res, "Container should be included based on mock config")

	// Verify that the mock config is used
	container = workloadmetafilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "excluded-container",
			},
		},
		nil,
	)
	res = filterBundle.IsExcluded(container)
	assert.Equal(t, true, res, "Container should be excluded based on mock config")
}
