// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

// Package systray
package systray

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: windows-agent

type Params struct {
	LaunchGuiFlag      bool
	LaunchElevatedFlag bool
	LaunchCommand      string
}

type Component interface {
}

var Module = fxutil.Component(
	fx.Provide(newSystray),
)
