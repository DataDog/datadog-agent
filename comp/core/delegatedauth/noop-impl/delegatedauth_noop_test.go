// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package delegatedauthimpl

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
)

func TestNoopAddInstance(t *testing.T) {
	noop := &delegatedAuthNoop{}
	err := noop.AddInstance(delegatedauth.InstanceParams{
		OrgUUID:         "test-org-uuid",
		RefreshInterval: 60,
		APIKeyConfigKey: "api_key",
	})
	assert.NoError(t, err)
}

func TestNoopStatusProviderName(t *testing.T) {
	noop := &delegatedAuthNoop{}
	assert.Equal(t, "Delegated Auth", noop.Name())
}

func TestNoopStatusProviderSection(t *testing.T) {
	noop := &delegatedAuthNoop{}
	assert.Equal(t, "delegatedauth", noop.Section())
}

func TestNoopStatusJSON(t *testing.T) {
	noop := &delegatedAuthNoop{}
	stats := make(map[string]interface{})
	err := noop.JSON(false, stats)

	require.NoError(t, err)
	assert.Equal(t, false, stats["enabled"])
}

func TestNoopStatusText(t *testing.T) {
	noop := &delegatedAuthNoop{}
	var buffer bytes.Buffer
	err := noop.Text(false, &buffer)

	require.NoError(t, err)
	output := buffer.String()
	assert.Contains(t, output, "Delegated Authentication is not enabled")
}

func TestNoopStatusHTML(t *testing.T) {
	noop := &delegatedAuthNoop{}
	var buffer bytes.Buffer
	err := noop.HTML(false, &buffer)

	require.NoError(t, err)
	output := buffer.String()
	assert.Contains(t, output, "Delegated Authentication")
	assert.Contains(t, output, "Not enabled")
	assert.Contains(t, output, "<div")
}

func TestNewComponent(t *testing.T) {
	provides := NewComponent()
	assert.NotNil(t, provides.Comp)
	assert.NotNil(t, provides.StatusProvider)

	// Verify component implements the interface
	var _ delegatedauth.Component = provides.Comp

	// Verify status provider works
	stats := make(map[string]interface{})
	err := provides.Comp.(*delegatedAuthNoop).JSON(false, stats)
	require.NoError(t, err)
	assert.Equal(t, false, stats["enabled"])
}
