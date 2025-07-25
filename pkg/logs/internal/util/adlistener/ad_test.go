// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package adlistener

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

//nolint:revive // TODO(AML) Fix revive linter
func TestListenersGetScheduleCalls(t *testing.T) {
	adsched := scheduler.NewControllerAndStart()
	ac := fxutil.Test[autodiscovery.Mock](t,
		fx.Supply(autodiscoveryimpl.MockParams{Scheduler: adsched}),
		secretsimpl.MockModule(),
		autodiscoveryimpl.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		core.MockBundle(),
		taggerfxmock.MockModule(),
		workloadfilterfxmock.MockModule(),
	)

	got1 := make(chan struct{}, 1)
	l1 := NewADListener("l1", ac, func(configs []integration.Config) {
		for range configs {
			got1 <- struct{}{}
		}
	}, nil)
	l1.StartListener()

	got2 := make(chan struct{}, 1)
	l2 := NewADListener("l2", ac, func(configs []integration.Config) {
		for range configs {
			got2 <- struct{}{}
		}
	}, nil)
	l2.StartListener()

	adsched.ApplyChanges(integration.ConfigChanges{Schedule: []integration.Config{{}}})

	// wait for each of the two listeners to get notified
	<-got1
	<-got2
}
