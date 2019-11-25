// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package loaders

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type LoaderOne struct{}

func (lo LoaderOne) Load(config integration.Config) ([]check.Check, error) { return nil, nil }

type LoaderTwo struct{}

func (lt LoaderTwo) Load(config integration.Config) ([]check.Check, error) { return nil, nil }

type LoaderThree struct{}

func (lt *LoaderThree) Load(config integration.Config) ([]check.Check, error) { return nil, nil }

func TestLoaderCatalog(t *testing.T) {
	l1 := LoaderOne{}
	factory1 := func() (check.Loader, error) { return l1, nil }
	l2 := LoaderTwo{}
	factory2 := func() (check.Loader, error) { return l2, nil }
	var l3 *LoaderThree
	factory3 := func() (check.Loader, error) { return l3, errors.New("error") }

	RegisterLoader(20, factory1)
	RegisterLoader(10, factory2)
	RegisterLoader(30, factory3)

	require.Len(t, LoaderCatalog(), 2)
	assert.Equal(t, l1, LoaderCatalog()[1])
	assert.Equal(t, l2, LoaderCatalog()[0])
}
