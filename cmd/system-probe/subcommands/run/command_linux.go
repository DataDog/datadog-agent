// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package run

import (
	"context"
	"time"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func setupRuntime(ctx context.Context) {
	runtime.SetMaxProcs()
	go func() {
		limiter, err := runtime.NewDynamicMemoryLimiter(1*time.Second, ddconfig.IsContainerized(), 0.20, func(ms cgroups.MemoryStats) uint64 {
			var nonGoMemory uint64
			if ms.KernelMemory != nil {
				nonGoMemory += *ms.KernelMemory
			}
			if ms.MappedFile != nil {
				nonGoMemory += *ms.MappedFile
			}
			return nonGoMemory
		})
		if err != nil {
			log.Infof("Creating memory limiter failed with: %v", err)
		}

		err = limiter.Run(ctx)
		if err != nil {
			log.Infof("Running memory limiter failed with: %v", err)
		}
	}()
}
