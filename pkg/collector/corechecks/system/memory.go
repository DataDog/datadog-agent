// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package system

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const memCheckName = "memory"

// Configure the Python check from YAML data
func (c *MemoryCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	return nil
}

func memFactory() check.Check {
	return &MemoryCheck{
		CheckBase: core.NewCheckBase(memCheckName),
	}
}
func init() {
	core.RegisterCheck(memCheckName, memFactory)
}
