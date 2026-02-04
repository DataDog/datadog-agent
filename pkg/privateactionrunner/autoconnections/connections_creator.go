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
	client ConnectionsClient
}

func NewConnectionsCreator(client ConnectionsClient) ConnectionsCreator {
	return ConnectionsCreator{client}
}

type AutoCreateConnectionsInput struct {
	ddSite     string
	runnerID   string
	runnerName string
	apiKey     string
	appKey     string
	allowlist  []string
}

func (c ConnectionsCreator) AutoCreateConnections(ctx context.Context, input AutoCreateConnectionsInput) error {
	definitions := DetermineConnectionsToCreate(input.allowlist)
	if len(definitions) == 0 {
		log.Info("No bundles in allowlist for auto-connection creation")
		return nil
	}

	for _, definition := range definitions {
		err := c.client.CreateConnection(ctx, definition, input.runnerID, input.runnerName)
		if err != nil {
			log.Warnf("Failed to create %s connection: %v", definition.IntegrationType, err)
		} else {
			log.Infof("Successfully created %s connection: %s (%s)",
				definition.IntegrationType, definition.IntegrationType, input.runnerID)
		}
	}

	return nil
}
