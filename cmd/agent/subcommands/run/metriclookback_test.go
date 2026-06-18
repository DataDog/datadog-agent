// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package run

import (
	"testing"

	"github.com/stretchr/testify/require"

	corechecks "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

func TestMetricLookbackShadowCoreLoaderUsesShadowMode(t *testing.T) {
	loader := newMetricLookbackShadowCoreLoader()

	coreLoader, ok := loader.(*corechecks.GoCheckLoader)
	require.True(t, ok)
	require.Equal(t, corechecks.GoCheckLoaderName, coreLoader.Name())
	require.Equal(t, corechecks.ShadowLoadMode, coreLoader.LoadMode())
}
