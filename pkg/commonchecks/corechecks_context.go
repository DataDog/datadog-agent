// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commonchecks

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func telemetryProviderForLoadMode(normalTelemetry telemetry.Component) func(corecheckLoader.ConstructionContext) telemetry.Component {
	shadowTelemetry := noopsimpl.NewComponent()
	return func(ctx corecheckLoader.ConstructionContext) telemetry.Component {
		if ctx.Mode == corecheckLoader.ShadowLoadMode {
			return shadowTelemetry
		}
		return normalTelemetry
	}
}

func contextualCoreFactory(factoryForContext func(corecheckLoader.ConstructionContext) option.Option[func() check.Check]) option.Option[func(corecheckLoader.ConstructionContext) check.Check] {
	normalFactory := factoryForContext(corecheckLoader.ConstructionContext{Mode: corecheckLoader.NormalLoadMode})
	_, ok := normalFactory.Get()
	if !ok {
		return option.None[func(corecheckLoader.ConstructionContext) check.Check]()
	}

	return option.New(func(ctx corecheckLoader.ConstructionContext) check.Check {
		factoryForMode := factoryForContext(ctx)
		factory, ok := factoryForMode.Get()
		if !ok {
			return nil
		}
		return factory()
	})
}
