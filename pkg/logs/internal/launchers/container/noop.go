// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package container

import (
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// noopLauncher does nothing.
type noopLauncher struct{}

// NewNoopLauncher returns a new noopLauncher.
func NewNoopLauncher() startstop.StartStoppable {
	return &noopLauncher{}
}

// Start does nothing.
func (l *noopLauncher) Start() {}

// Stop does nothing.
func (l *noopLauncher) Stop() {}
