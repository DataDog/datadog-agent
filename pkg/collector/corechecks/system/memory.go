// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package system

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
)

const memCheckName = "memory"

func memFactory() check.Check {
	return &MemoryCheck{
		CheckBase: core.NewCheckBase(memCheckName),
	}
}
func init() {
	core.RegisterCheck(memCheckName, memFactory)
}
