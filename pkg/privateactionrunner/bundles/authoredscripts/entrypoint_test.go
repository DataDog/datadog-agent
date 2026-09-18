// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type entrypointTestRCClient struct {
	product string
	handler func(map[string]state.RawConfig, func(string, state.ApplyStatus))
}

func (c *entrypointTestRCClient) Subscribe(product string, handler func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
	c.product = product
	c.handler = handler
}

func (*entrypointTestRCClient) GetConfigTUFProof(string) (state.ConfigTUFProof, bool) {
	return state.ConfigTUFProof{}, false
}

func TestAuthoredScriptsGetAction(t *testing.T) {
	client := &entrypointTestRCClient{}
	bundle, err := NewAuthoredScripts(client)
	require.NoError(t, err)
	require.Equal(t, state.ProductUpdaterCatalogDD, client.product)
	require.NotNil(t, client.handler)

	handler := bundle.GetAction("addRepo")

	assert.IsType(t, &RunAuthoredScriptHandler{}, handler)
	assert.Same(t, handler, bundle.GetAction("restartService"))
	assert.Nil(t, bundle.GetAction(""))
}
