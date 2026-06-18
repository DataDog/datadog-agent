// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package run

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	corechecks "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

func TestNoTelemetryGPUShadowLoaderMetadata(t *testing.T) {
	loader := noTelemetryGPUShadowLoader{}

	require.Equal(t, corechecks.GoCheckLoaderName, loader.Name())
	require.Equal(t, "Metric Lookback GPU Shadow Loader", loader.String())
}

func TestNoTelemetryGPUShadowLoaderSkipsNonGPUChecks(t *testing.T) {
	loader := noTelemetryGPUShadowLoader{}

	loadedCheck, err := loader.Load(nil, integration.Config{Name: "cpu"}, nil, 0)

	require.Nil(t, loadedCheck)
	require.ErrorContains(t, err, "not a gpu check")
}
