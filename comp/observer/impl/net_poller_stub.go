//go:build !linux

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// NetPoller is a no-op on non-linux platforms.
type NetPoller struct{}

// NetPollerConfig holds configuration for the network poller.
type NetPollerConfig struct {
	Interval time.Duration
	ProcPath string
}

// NewNetPoller returns a no-op poller on non-linux platforms.
func NewNetPoller(_ observerdef.Handle, _ NetPollerConfig) *NetPoller {
	return &NetPoller{}
}

// Start is a no-op on non-linux.
func (p *NetPoller) Start() {}

// Stop is a no-op on non-linux.
func (p *NetPoller) Stop() {}
