// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package memory

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"

const memCheckName = "memory"

func memFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(memCheckName),
	}
}
func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\collector\corechecks\system\memory\memory.go 20`)
	core.RegisterCheck(memCheckName, memFactory)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\collector\corechecks\system\memory\memory.go 21`)
}
