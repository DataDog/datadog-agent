// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package cloudfoundrycontainerimpl provides the implementation of the cloud foundry container component.
package cloudfoundrycontainerimpl

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/agent/cloudfoundrycontainer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cloudfoundrycontainertagger "github.com/DataDog/datadog-agent/pkg/cloudfoundry/containertagger"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	pkgcommon "github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newCloudfoundryContainer),
	)
}

type dependencies struct {
	fx.In
	Config config.Component // Don't remove Config as it must be loaded before using IsFeaturePresent
	WMeta  workloadmeta.Component
	LC     fx.Lifecycle
}

func newCloudfoundryContainer(deps dependencies) cloudfoundrycontainer.Component {
	// start the cloudfoundry container tagger
	if pkgconfig.IsFeaturePresent(pkgconfig.CloudFoundry) && !deps.Config.GetBool("cloud_foundry_buildpack") {
		containerTagger, err := cloudfoundrycontainertagger.NewContainerTagger(deps.WMeta)
		if err != nil {
			log.Errorf("Failed to create Cloud Foundry container tagger: %v", err)
		} else {
			ctx, cancel := pkgcommon.GetMainCtxCancel()
			deps.LC.Append(fx.Hook{
				OnStart: func(_ context.Context) error {
					containerTagger.Start(ctx)
					return nil
				},
				OnStop: func(_ context.Context) error {
					cancel()
					return nil
				},
			})
		}
	}
	return struct{}{}
}
