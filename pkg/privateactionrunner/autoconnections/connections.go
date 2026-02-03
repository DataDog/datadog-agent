// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getBundleKeyForDefinition returns the bundle key for a given definition
// TODO: Used for metric tagging
func getBundleKeyForDefinition(def ConnectionDefinition) string {
	for key, definition := range supportedConnections {
		if definition.BundleID == def.BundleID {
			return key
		}
	}
	return "unknown"
}

type AutoCreateConnectionsInput struct {
	cfg        model.Reader
	ddSite     string
	runnerID   string
	runnerName string
	apiKey     string
	appKey     string
	allowlist  []string
	client     *ConnectionsClient
}

func AutoCreateConnections(ctx context.Context, input AutoCreateConnectionsInput) error {
	definitions := DetermineConnectionsToCreate(input.allowlist)
	if len(definitions) == 0 {
		log.Info("No bundles in allowlist for auto-connection creation")
		return nil
	}

	for _, definition := range definitions {
		err := input.client.CreateConnection(ctx, definition, input.runnerID, input.runnerName)
		if err != nil {
			log.Warnf("Failed to create %s connection: %v", definition.IntegrationType, err)
		} else {
			log.Infof("Successfully created %s connection: %s (%s)",
				definition.IntegrationType, definition.IntegrationType, input.runnerID)
		}
	}

	return nil
}
