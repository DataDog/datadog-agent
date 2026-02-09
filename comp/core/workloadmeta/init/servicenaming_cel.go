// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel && servicenaming

package init

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/servicenaming/subscriber"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// initServiceNaming initializes the CEL-based service naming subscriber if the feature is enabled in the configuration.
func initServiceNaming(ctx context.Context, wm workloadmeta.Component, cfg config.Component) error {
	serviceNamingExplicitlyEnabled := cfg.GetBool("service_discovery.enabled")
	serviceNamingSub, err := subscriber.NewSubscriber(cfg, wm)

	if err != nil {
		// Handle initialization errors: block startup if explicitly enabled, otherwise just log a debug message.
		if serviceNamingExplicitlyEnabled {
			return fmt.Errorf("CEL service naming is enabled but failed to initialize: %w", err)
		}
		log.Debugf("CEL service naming subscriber not initialized: %v", err)
	} else if serviceNamingSub != nil {
		// Start the subscriber in a separate goroutine if it was successfully instantiated.
		go serviceNamingSub.Start(ctx)
		log.Infof("CEL service naming subscriber started successfully")
	} else {
		// Case where no error occurred but the subscriber is nil (e.g., feature disabled or no rules defined).
		log.Debug("CEL service naming subscriber not created (disabled or no rules configured)")
	}

	return nil
}
