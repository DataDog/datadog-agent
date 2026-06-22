// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package run

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	corechecks "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

func TestMetricLookbackShadowCoreLoaderUsesShadowMode(t *testing.T) {
	loader := newMetricLookbackShadowCoreLoader()

	coreLoader, ok := loader.(*corechecks.GoCheckLoader)
	require.True(t, ok)
	require.Equal(t, corechecks.GoCheckLoaderName, coreLoader.Name())
	require.Equal(t, corechecks.ShadowLoadMode, coreLoader.LoadMode())
}

func TestMetricLookbackShadowLoadersReplaceCoreLoaderInPlace(t *testing.T) {
	normalCoreLoader, err := corechecks.NewGoCheckLoader()
	require.NoError(t, err)
	catalog := []check.Loader{
		metricLookbackTestLoader{name: "python"},
		normalCoreLoader,
		metricLookbackTestLoader{name: "jmx"},
	}

	shadowLoaders := newMetricLookbackShadowLoadersFromCatalog(catalog)

	require.Len(t, shadowLoaders, len(catalog))
	require.Equal(t, "python", shadowLoaders[0].Name())
	require.Equal(t, "jmx", shadowLoaders[2].Name())

	coreLoader, ok := shadowLoaders[1].(*corechecks.GoCheckLoader)
	require.True(t, ok)
	require.NotSame(t, normalCoreLoader, coreLoader)
	require.Equal(t, corechecks.ShadowLoadMode, coreLoader.LoadMode())

	coreLoaderCount := 0
	for _, loader := range shadowLoaders {
		if _, ok := loader.(*corechecks.GoCheckLoader); ok {
			coreLoaderCount++
		}
	}
	require.Equal(t, 1, coreLoaderCount)
}

func TestMetricLookbackShadowLoadersAppendShadowCoreLoaderWhenCatalogHasNoCoreLoader(t *testing.T) {
	catalog := []check.Loader{
		metricLookbackTestLoader{name: "python"},
		metricLookbackTestLoader{name: "jmx"},
	}

	shadowLoaders := newMetricLookbackShadowLoadersFromCatalog(catalog)

	require.Len(t, shadowLoaders, len(catalog)+1)
	require.Equal(t, "python", shadowLoaders[0].Name())
	require.Equal(t, "jmx", shadowLoaders[1].Name())

	coreLoader, ok := shadowLoaders[2].(*corechecks.GoCheckLoader)
	require.True(t, ok)
	require.Equal(t, corechecks.ShadowLoadMode, coreLoader.LoadMode())
}

type metricLookbackTestLoader struct {
	name string
}

func (l metricLookbackTestLoader) Name() string {
	return l.name
}

func (metricLookbackTestLoader) Load(sender.SenderManager, integration.Config, integration.Data, int) (check.Check, error) {
	return nil, errors.New("not implemented")
}
