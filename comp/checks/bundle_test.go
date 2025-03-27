// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package checks

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	authtokenmock "github.com/DataDog/datadog-agent/comp/api/authtoken/mock"
	"github.com/DataDog/datadog-agent/comp/core"
	agenttelemetryfx "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/crashreport"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	fxutil.TestBundle(t, Bundle(),
		core.MockBundle(),
		fx.Provide(func(t testing.TB) authtoken.Component { return authtokenmock.New(t) }),
		fx.Provide(func() tagger.Component { return fakeTagger }),
		fx.Supply(core.BundleParams{}),
		agenttelemetryfx.Module(),
		fx.Supply(crashreport.WinCrashReporter{}),
	)
}
