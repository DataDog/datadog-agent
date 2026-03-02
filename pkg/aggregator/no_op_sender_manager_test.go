// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

func TestNewNoOpSenderManager(t *testing.T) {
	manager := NewNoOpSenderManager()
	assert.NotNil(t, manager)
}

func TestNoOpSenderManagerGetSender(t *testing.T) {
	manager := NewNoOpSenderManager()
	sender, err := manager.GetSender(checkid.ID("test-check"))
	assert.Nil(t, sender)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NotImplemented")
}

func TestNoOpSenderManagerSetSender(t *testing.T) {
	manager := NewNoOpSenderManager()
	err := manager.SetSender(nil, checkid.ID("test-check"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NotImplemented")
}

func TestNoOpSenderManagerDestroySender(t *testing.T) {
	manager := NewNoOpSenderManager()
	// DestroySender should not panic and does nothing
	manager.DestroySender(checkid.ID("test-check"))
}

func TestNoOpSenderManagerGetDefaultSender(t *testing.T) {
	manager := NewNoOpSenderManager()
	sender, err := manager.GetDefaultSender()
	assert.Nil(t, sender)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NotImplemented")
}
