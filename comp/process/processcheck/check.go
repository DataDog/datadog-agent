// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processcheck

import (
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

var _ types.CheckComponent = (*check)(nil)

type check struct {
	processCheck *checks.ProcessCheck
}

func newCheck() types.ProvidesCheck {
	return types.ProvidesCheck{
		CheckComponent: &check{
			processCheck: checks.NewProcessCheck(),
		},
	}
}

func (c *check) Object() checks.Check {
	return c.processCheck
}
