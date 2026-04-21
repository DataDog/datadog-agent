// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cpu

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/shirou/gopsutil/v4/load"
)

// GetContextSwitches retrieves the number of context switches for the current process.
// It returns an integer representing the count and an error if the retrieval fails.
func GetContextSwitches() (int64, error) {
	log.Debug("collecting ctx switches")
	misc, err := load.Misc()
	if err != nil {
		return 0, err
	}
	return int64(misc.Ctxt), nil
}
