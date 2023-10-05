// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package checks

import (
	"testing"

	compsysconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	comptraceconfig "github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/crashreport"
	"go.uber.org/fx"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(Bundle,
		comptraceconfig.Module,
		core.MockBundle,
		compsysconfig.Module,
		fx.Supply(crashreport.WinCrashReporter{}),
		fx.Supply(compsysconfig.Params{}),
	)
}
