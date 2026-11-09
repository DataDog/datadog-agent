// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package instrumentation provides the DatadogInstrumentation controller and product handlers.
package instrumentation

import (
	"go.uber.org/fx"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	apm "github.com/DataDog/datadog-agent/comp/instrumentation/apm/def"
	apmfx "github.com/DataDog/datadog-agent/comp/instrumentation/apm/fx"
	checks "github.com/DataDog/datadog-agent/comp/instrumentation/checks/def"
	checksfx "github.com/DataDog/datadog-agent/comp/instrumentation/checks/fx"
	"github.com/DataDog/datadog-agent/comp/instrumentation/common"
	controllerfx "github.com/DataDog/datadog-agent/comp/instrumentation/controller/fx"
	logs "github.com/DataDog/datadog-agent/comp/instrumentation/logs/def"
	logsfx "github.com/DataDog/datadog-agent/comp/instrumentation/logs/fx"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: container-platform

// Bundle provides the controller, product handlers, and their shared stores.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		controllerfx.Module(),
		checksfx.Module(),
		logsfx.Module(),
		apmfx.Module(),
		fx.Provide(common.NewCheckStore, common.NewServiceCheckTemplateStore, common.NewAPMTargetStore),
		fx.Provide(func(s *common.CheckStore) common.ConfigWriter { return s }),
		fx.Provide(func(s *common.ServiceCheckTemplateStore) common.ServiceTemplateWriter { return s }),
		fx.Provide(func(s *common.ServiceCheckTemplateStore) adtypes.ServiceTracker { return s }),
		fx.Provide(func(checks checks.Component, logs logs.Component, apm apm.Component) []instrumentation.Handler {
			return []instrumentation.Handler{checks, logs, apm}
		}),
	)
}
