// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	"net/http"
	"testing"

	rcmock "github.com/DataDog/datadog-agent/pkg/config/remote/client/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthoredScriptsGetAction(t *testing.T) {
	client := rcmock.New()
	bundle, err := NewAuthoredScripts(client, http.DefaultClient)
	require.NoError(t, err)
	require.Positive(t, client.Subscribers(state.ProductUpdaterCatalogDD))

	handler := bundle.GetAction("addRepo")

	assert.IsType(t, &RunAuthoredScriptHandler{}, handler)
	assert.Same(t, handler, bundle.GetAction("restartService"))
	assert.Nil(t, bundle.GetAction(""))
}
