// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PLINT) Fix revive linter
package memory

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const memCheckName = "memory"

func memFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(memCheckName),
	}
}
func init() {
	core.RegisterCheck(memCheckName, memFactory)
}
