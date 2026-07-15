// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commonchecks

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestTelemetryProviderForLoadMode(t *testing.T) {
	normalTelemetry := telemetryimpl.NewMockComponent()
	provider := telemetryProviderForLoadMode(normalTelemetry)

	require.Equal(t, componentPointer(normalTelemetry), componentPointer(provider(corecheckLoader.ConstructionContext{Mode: corecheckLoader.NormalLoadMode})))

	shadowTelemetry := provider(corecheckLoader.ConstructionContext{Mode: corecheckLoader.ShadowLoadMode})
	require.NotEqual(t, componentPointer(normalTelemetry), componentPointer(shadowTelemetry))
	require.Equal(t, componentPointer(shadowTelemetry), componentPointer(provider(corecheckLoader.ConstructionContext{Mode: corecheckLoader.ShadowLoadMode})))
}

func componentPointer(component any) uintptr {
	return reflect.ValueOf(component).Pointer()
}

func TestContextualCoreFactoryEvaluatesContextAtConstructionTime(t *testing.T) {
	var modes []corecheckLoader.LoadMode
	contextualFactory := contextualCoreFactory(func(ctx corecheckLoader.ConstructionContext) option.Option[func() check.Check] {
		modes = append(modes, ctx.Mode)
		return option.New(func() check.Check {
			return &stub.StubCheck{}
		})
	})

	factory, ok := contextualFactory.Get()
	require.True(t, ok)

	loadedCheck := factory(corecheckLoader.ConstructionContext{Mode: corecheckLoader.ShadowLoadMode})

	require.NotNil(t, loadedCheck)
	require.Equal(t, []corecheckLoader.LoadMode{corecheckLoader.NormalLoadMode, corecheckLoader.ShadowLoadMode}, modes)
}
