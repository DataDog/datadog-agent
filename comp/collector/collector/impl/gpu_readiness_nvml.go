// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package collectorimpl

import (
	"github.com/benbjohnson/clock"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
)

func (c *collectorImpl) registerGPUReadiness(lc compdef.Lifecycle) {
	c.registerNVMLReadiness(lc, safenvml.HasInitialized, clock.New())
}
