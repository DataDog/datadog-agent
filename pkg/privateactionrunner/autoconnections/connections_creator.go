// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ConnectionsCreator struct {
	client   ConnectionsClient
	provider TagsProvider
}

func NewConnectionsCreator(client ConnectionsClient, provider TagsProvider) ConnectionsCreator {
	return ConnectionsCreator{
		client:   client,
		provider: provider,
	}
}

func (c ConnectionsCreator) AutoCreateConnections(ctx context.Context, runnerID string, enrollmentResult *enrollment.Result, actionsAllowlist []string) error {
	definitions := DetermineConnectionsToCreate(actionsAllowlist)
	if len(definitions) == 0 {
		log.Info("No actions in actions_allowlist for auto-connection creation")
		return nil
	}

	tags := c.provider.GetTags(ctx, runnerID, enrollmentResult.Hostname)

	for _, definition := range definitions {
		err := c.client.CreateConnection(ctx, definition, runnerID, enrollmentResult.RunnerName, tags)
		if err != nil {
			log.Warnf("Failed to create %s connection: %v", definition.IntegrationType, err)
		} else {
			log.Infof("Successfully created %s connection: %s (%s)",
				definition.IntegrationType, definition.IntegrationType, runnerID)
		}
	}

	return nil
}
