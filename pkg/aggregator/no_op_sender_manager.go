// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// NoOpSenderManager is an empty sender.SenderManager that returns NotImplemented error when calling its methods.
type NoOpSenderManager struct{}

// NewNoOpSenderManager creates a new instance of NoOpSenderManager.
func NewNoOpSenderManager() NoOpSenderManager {
	return NoOpSenderManager{}
}

// GetSender returns a sender.Sender with passed ID
func (NoOpSenderManager) GetSender(id checkid.ID) (sender.Sender, error) {
	return nil, errors.New("NotImplemented")
}

// SetSender returns the passed sender with the passed ID.
func (NoOpSenderManager) SetSender(sender.Sender, checkid.ID) error {
	return errors.New("NotImplemented")
}

// DestroySender frees up the resources used by the sender with passed ID (by deregistering it from the aggregator)
func (NoOpSenderManager) DestroySender(id checkid.ID) {}

// GetDefaultSender returns a default sender.
func (NoOpSenderManager) GetDefaultSender() (sender.Sender, error) {
	return nil, errors.New("NotImplemented")
}
