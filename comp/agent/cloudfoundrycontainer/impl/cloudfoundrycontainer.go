// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package cloudfoundrycontainerimpl provides the implementation of the cloud foundry container component.
package cloudfoundrycontainerimpl

import (
	"context"

	cloudfoundrycontainer "github.com/DataDog/datadog-agent/comp/agent/cloudfoundrycontainer/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	cloudfoundrycontainertagger "github.com/DataDog/datadog-agent/pkg/cloudfoundry/containertagger"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgcommon "github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the cloudfoundrycontainer component.
type Requires struct {
	Config config.Component // Don't remove Config as it must be loaded before using IsFeaturePresent
	WMeta  workloadmeta.Component
	LC     compdef.Lifecycle
}

// Provides defines the output of the cloudfoundrycontainer component.
type Provides struct {
	Comp cloudfoundrycontainer.Component
}

// NewComponent creates a new cloudfoundrycontainer component.
func NewComponent(reqs Requires) Provides {
	// start the cloudfoundry container tagger
	if env.IsFeaturePresent(env.CloudFoundry) && !reqs.Config.GetBool("cloud_foundry_buildpack") {
		containerTagger, err := cloudfoundrycontainertagger.NewContainerTagger(reqs.WMeta)
		if err != nil {
			log.Errorf("Failed to create Cloud Foundry container tagger: %v", err)
		} else {
			ctx, cancel := pkgcommon.GetMainCtxCancel()
			reqs.LC.Append(compdef.Hook{
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
	return Provides{Comp: struct{}{}}
}
