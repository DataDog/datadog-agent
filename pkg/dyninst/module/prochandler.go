// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
)

type processHandler struct {
	controller     *Controller
	actuator       *actuator.Tenant
	scraperHandler procmon.Handler
}

var _ procmon.Handler = (*processHandler)(nil)

func (c *processHandler) HandleUpdate(update procmon.ProcessesUpdate) {
	c.controller.store.remove(update.Removals)
	if len(update.Removals) > 0 {
		c.actuator.HandleUpdate(actuator.ProcessesUpdate{
			Removals: update.Removals,
		})
	}
	c.scraperHandler.HandleUpdate(update)
}
