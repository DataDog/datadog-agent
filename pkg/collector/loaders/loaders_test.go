// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loaders

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

type LoaderOne struct{}

func (lo LoaderOne) Name() string {
	return "loader_one"
}

func (lo LoaderOne) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data) (check.Check, error) {
	var c check.Check
	return c, nil
}

type LoaderTwo struct{}

func (lt LoaderTwo) Name() string {
	return "loader_two"
}

func (lt LoaderTwo) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data) (check.Check, error) {
	var c check.Check
	return c, nil
}

type LoaderThree struct{}

func (lt *LoaderThree) Name() string {
	return "loader_three"
}

func (lt *LoaderThree) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data) (check.Check, error) {
	var c check.Check
	return c, nil
}

func TestLoaderCatalog(t *testing.T) {
	l1 := LoaderOne{}
	factory1 := func(sender.SenderManager) (check.Loader, error) { return l1, nil }
	l2 := LoaderTwo{}
	factory2 := func(sender.SenderManager) (check.Loader, error) { return l2, nil }
	var l3 *LoaderThree
	factory3 := func(sender.SenderManager) (check.Loader, error) { return l3, errors.New("error") }

	RegisterLoader(20, factory1)
	RegisterLoader(10, factory2)
	RegisterLoader(30, factory3)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	require.Len(t, LoaderCatalog(senderManager), 2)
	assert.Equal(t, l1, LoaderCatalog(senderManager)[1])
	assert.Equal(t, l2, LoaderCatalog(senderManager)[0])
}
