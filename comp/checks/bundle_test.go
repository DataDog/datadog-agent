// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package checks

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	comptraceconfig "github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/crashreport"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	fakeTagger := mock.SetupFakeTagger(t)

	fxutil.TestBundle(t, Bundle(),
		comptraceconfig.Module(),
		core.MockBundle(),
		fx.Provide(func() tagger.Component { return fakeTagger }),
		fx.Supply(core.BundleParams{}),
		fx.Supply(crashreport.WinCrashReporter{}),
	)
}
