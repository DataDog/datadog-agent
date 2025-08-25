// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
)

type processHandler struct {
	controller     *Controller
	scraperHandler procmon.Handler
}

var _ procmon.Handler = (*processHandler)(nil)

func (c *processHandler) HandleUpdate(update procmon.ProcessesUpdate) {
	c.controller.handleRemovals(update.Removals)
	c.scraperHandler.HandleUpdate(update)
}
