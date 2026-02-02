// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

package init

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/servicenaming/subscriber"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// initServiceNaming initializes the CEL-based service naming subscriber if enabled.
// This function is only available when the 'cel' build tag is set.
func initServiceNaming(ctx context.Context, wm workloadmeta.Component, cfg config.Component) error {
	serviceNamingExplicitlyEnabled := cfg.GetBool("service_discovery.enabled")
	serviceNamingSub, err := subscriber.NewSubscriber(cfg, wm)
	if err != nil {
		// Fail fast if user explicitly enabled the feature
		if serviceNamingExplicitlyEnabled {
			return fmt.Errorf("CEL service naming is enabled but failed to initialize: %w", err)
		}
		log.Debugf("CEL service naming subscriber not initialized: %v", err)
	} else if serviceNamingSub != nil {
		go serviceNamingSub.Start(ctx)
		log.Infof("CEL service naming subscriber started successfully")
	}

	return nil
}
