// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
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

// CreateConnectionsIfEnabled runs the best-effort connection auto-creation flow.
func CreateConnectionsIfEnabled(
	ctx context.Context,
	cfg model.Reader,
	parCfg *parconfig.Config,
	apiKey, appKey, runnerID string,
	enrollmentResult *enrollment.Result,
	tagsProvider TagsProvider,
) {
	if cfg.GetBool(setup.PARApiKeyOnlyEnrollment) {
		return
	}
	if cfg.GetBool(setup.PARSkipConnectionCreation) {
		return
	}

	actionsAllowlist := make([]string, 0, len(parCfg.ActionsAllowlist))
	for fqnPrefix := range parCfg.ActionsAllowlist {
		actionsAllowlist = append(actionsAllowlist, fqnPrefix)
	}
	if len(actionsAllowlist) == 0 {
		return
	}

	client, err := NewConnectionsAPIClient(cfg, parCfg.DatadogSite, apiKey, appKey)
	if err != nil {
		log.Warnf("Failed to create connections API client: %v", err)
		return
	}
	creator := NewConnectionsCreator(*client, tagsProvider)
	if err := creator.AutoCreateConnections(ctx, runnerID, enrollmentResult, actionsAllowlist); err != nil {
		log.Warnf("Failed to auto-create connections: %v", err)
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
