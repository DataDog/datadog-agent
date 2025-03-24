// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package gpusubscriberimpl subscribes to GPU events
package gpusubscriberimpl

import (
	"github.com/DataDog/datadog-agent/comp/process/gpusubscriber/def"
)

// NewComponent returns a new gpu subscriber.
func NewComponent() gpusubscriber.Component {
	return &NoopSubscriber{}
}
