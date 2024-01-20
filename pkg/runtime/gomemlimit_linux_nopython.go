// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !python

package runtime

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// RunMemoryLimiterFromConfig runs the memory limiter, filling args from the config
func RunMemoryLimiterFromConfig(c context.Context, configNS string) error {
	return RunMemoryLimiter(c, MemoryLimiterArgs{
		LimitPct: config.Datadog.GetFloat64(setup.NSKey(configNS, "go_memlimit_pct")),
	})
}

// RunMemoryLimiter runs the memory limiter
func RunMemoryLimiter(c context.Context, args MemoryLimiterArgs) error {
	return NewStaticMemoryLimiter(args.LimitPct, args.MaxMemory, env.IsContainerized()).Run(c)
}
