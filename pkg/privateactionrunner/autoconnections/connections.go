// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getBundleKeyForDefinition returns the bundle key for a given definition
// Used for metric tagging
func getBundleKeyForDefinition(def ConnectionDefinition) string {
	for key, definition := range SupportedConnections {
		if definition.BundleID == def.BundleID {
			return key
		}
	}
	return "unknown"
}

func AutoCreateConnections(ctx context.Context, cfg model.Reader, runnerURN string, allowlist []string) error {
	runnerID, err := extractRunnerIDFromURN(runnerURN)
	if err != nil {
		return fmt.Errorf("failed to extract runner ID: %w", err)
	}

	definitions := DetermineConnectionsToCreate(allowlist)
	if len(definitions) == 0 {
		log.Info("No bundles in allowlist for auto-connection creation")
		return nil
	}

	client, err := NewConnectionAPIClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	for _, definition := range definitions {
		_ = getBundleKeyForDefinition(definition)

		err := client.CreateConnection(ctx, definition, runnerID)
		if err != nil {
			log.Warnf("Failed to create %s connection: %v", definition.IntegrationType, err)
		} else {
			log.Infof("Successfully created %s connection: %s (%s)",
				definition.IntegrationType, definition.IntegrationType, runnerID)
		}
	}

	return nil
}
