// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cpu

import (
	"context"

	"github.com/shirou/gopsutil/v4/common"
	"github.com/shirou/gopsutil/v4/load"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetContextSwitches retrieves the number of context switches for the current process.
// It returns an integer representing the count and an error if the retrieval fails.
func GetContextSwitches() (int64, error) {
	log.Debug("collecting ctx switches")
	ctx := context.Background()
	if procfsPath := pkgconfigsetup.Datadog().GetString("procfs_path"); procfsPath != "" {
		ctx = context.WithValue(ctx, common.EnvKey, common.EnvMap{common.HostProcEnvKey: procfsPath})
	}
	misc, err := load.MiscWithContext(ctx)
	if err != nil {
		return 0, err
	}
	return int64(misc.Ctxt), nil
}
