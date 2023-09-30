// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/checks/agentcrashdetect"
	"github.com/DataDog/datadog-agent/comp/core/config"
	compsysconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	comptraceconfig "github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/crashreport"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		// instantiate all of the check components, since this is not done
		// automatically.
		comptraceconfig.Module,
		config.Module,
		compsysconfig.Module,
		fx.Supply(crashreport.WinCrashReporter{}),
		fx.Supply(config.Params{}),
		fx.Supply(compsysconfig.Params{}),
		fx.Invoke(func(agentcrashdetect.Component) {}),
		Bundle,
	))
}
