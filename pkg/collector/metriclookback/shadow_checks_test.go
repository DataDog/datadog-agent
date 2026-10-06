// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

type namedShadowLoader struct {
	check.Loader
	name string
}

func (l *namedShadowLoader) Name() string { return l.name }

func TestNewShadowCheckFactoryOmitsMissingSender(t *testing.T) {
	require.Nil(t, NewShadowCheckFactory(nil, nil))
}

func TestShadowLoaderForUnsupportedLoader(t *testing.T) {
	f := shadowCheckFactory{}
	loader, ok := f.shadowLoaderFor(&namedShadowLoader{name: "sharedlibrary"})
	require.False(t, ok)
	require.Nil(t, loader)
}

func TestShadowLoaderForPythonReusesLoadedLoader(t *testing.T) {
	loader := &namedShadowLoader{name: "python"}
	s := shadowCheckFactory{}

	shadowLoader, ok := s.shadowLoaderFor(loader)

	require.True(t, ok)
	assert.Same(t, loader, shadowLoader)
}

func TestShadowLoaderForCoreUsesShadowLoadMode(t *testing.T) {
	loader, err := core.NewGoCheckLoader()
	require.NoError(t, err)
	s := shadowCheckFactory{}

	shadowLoader, ok := s.shadowLoaderFor(loader)

	require.True(t, ok)
	shadowCoreLoader, ok := shadowLoader.(*core.GoCheckLoader)
	require.True(t, ok)
	assert.Equal(t, core.ShadowLoadMode, shadowCoreLoader.LoadMode())
}

func TestShadowLoaderForCoreReusesShadowLoader(t *testing.T) {
	loader, err := core.NewGoCheckLoader()
	require.NoError(t, err)
	s := shadowCheckFactory{}

	firstShadowLoader, ok := s.shadowLoaderFor(loader)
	require.True(t, ok)
	secondShadowLoader, ok := s.shadowLoaderFor(loader)

	require.True(t, ok)
	assert.Same(t, firstShadowLoader, secondShadowLoader)
}
