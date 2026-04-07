// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package hostinfomock provides a mock hostinfo component.
package hostinfomock

import (
	"go.uber.org/fx"

	hostinfoComp "github.com/DataDog/datadog-agent/comp/process/hostinfo/def"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type hostinfo struct {
	hostinfo *checks.HostInfo
}

func (h *hostinfo) Object() *checks.HostInfo {
	return h.hostinfo
}

func newMockHostInfo() hostinfoComp.Component {
	return &hostinfo{hostinfo: &checks.HostInfo{}}
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockHostInfo))
}
