// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux || !nvml

package collectorimpl

import compdef "github.com/DataDog/datadog-agent/comp/def"

func (c *collectorImpl) registerGPUReadiness(_ compdef.Lifecycle) {}
