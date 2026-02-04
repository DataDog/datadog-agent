// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ConnectionsCreator struct {
	client       ConnectionsClient
	configWriter ConfigWriter
}

func NewConnectionsCreator(client ConnectionsClient) ConnectionsCreator {
	return ConnectionsCreator{client, NewDefaultConfigWriter()}
}

func (c ConnectionsCreator) AutoCreateConnections(ctx context.Context, runnerID, runnerName string, allowlist []string) error {
	definitions := DetermineConnectionsToCreate(allowlist)
	if len(definitions) == 0 {
		log.Info("No bundles in allowlist for auto-connection creation")
		return nil
	}

	for _, definition := range definitions {
		// First, ensure required config files exist
		if err := c.ensureConfigFileForConnection(definition); err != nil {
			log.Warnf("Failed to ensure config file for %s, skipping connection creation: %v", definition.IntegrationType, err)
			continue // Skip this connection if config file creation failed
		}

		// Only create connection if config file creation succeeded (or wasn't needed)
		err := c.client.CreateConnection(ctx, definition, runnerID, runnerName)
		if err != nil {
			log.Warnf("Failed to create %s connection: %v", definition.IntegrationType, err)
		} else {
			log.Infof("Successfully created %s connection: %s (%s)",
				definition.IntegrationType, definition.IntegrationType, runnerID)
		}
	}

	return nil
}

// ensureConfigFileForConnection creates required configuration files for a connection
func (c ConnectionsCreator) ensureConfigFileForConnection(def ConnectionDefinition) error {
	switch def.IntegrationType {
	case "Script":
		created, err := c.configWriter.EnsureScriptBundleConfig()
		if err != nil {
			return err
		}
		if created {
			log.Infof("Auto-created configuration file for %s connection at %s",
				def.IntegrationType, GetScriptConfigPath())
		}
		return nil
	default:
		return nil
	}
}
