// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package mocksender

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// ErrStoppedSenderManager is the error returned by all StoppedSenderManager methods.
var ErrStoppedSenderManager = errors.New("demultiplexer is stopped")

// StoppedSenderManager is a fake sender.SenderManager that behaves like an
// already-stopped demultiplexer: every method returns ErrStoppedSenderManager.
type StoppedSenderManager struct{}

var _ sender.SenderManager = (*StoppedSenderManager)(nil)

// NewStoppedSenderManager returns a fake SenderManager that reports itself as stopped.
func NewStoppedSenderManager() *StoppedSenderManager { return &StoppedSenderManager{} }

// GetSender always reports the demultiplexer as stopped.
func (*StoppedSenderManager) GetSender(checkid.ID) (sender.Sender, error) {
	return nil, ErrStoppedSenderManager
}

// SetSender always reports the demultiplexer as stopped.
func (*StoppedSenderManager) SetSender(sender.Sender, checkid.ID) error {
	return ErrStoppedSenderManager
}

// DestroySender is a no-op on a stopped demultiplexer.
func (*StoppedSenderManager) DestroySender(checkid.ID) {}

// GetDefaultSender always reports the demultiplexer as stopped.
func (*StoppedSenderManager) GetDefaultSender() (sender.Sender, error) {
	return nil, ErrStoppedSenderManager
}
