// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import "github.com/DataDog/datadog-agent/pkg/util/startstop"

var (
	_ startstop.StartStoppable = &noOpRunnable{}
)

// noOpRunnable is a no-op implementation of startstop.StartStoppable.
type noOpRunnable struct{}

func (s noOpRunnable) Start() {}
func (s noOpRunnable) Stop()  {}
