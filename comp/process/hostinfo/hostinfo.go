// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostinfo

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

type dependencies struct {
	fx.In

	Config config.Component
	Logger log.Component
}

type hostinfo struct {
	hostinfo *checks.HostInfo
}

func newHostInfo(deps dependencies) (Component, error) {
	hinfo, err := checks.CollectHostInfo(deps.Config)
	if err != nil {
		_ = deps.Logger.Critical("Error collecting host details:", err)
		return nil, fmt.Errorf("error collecting host details: %v", err)
	}
	return &hostinfo{hostinfo: hinfo}, nil
}

func (h *hostinfo) Object() *checks.HostInfo {
	return h.hostinfo
}

func newMockHostInfo() Component {
	return &hostinfo{hostinfo: &checks.HostInfo{}}
}
