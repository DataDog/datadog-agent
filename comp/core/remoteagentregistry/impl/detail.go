// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type remoteAgentDetails struct {
	lastSeen     time.Time
	displayName  string
	apiEndpoint  string
	client       pb.RemoteAgentClient
	configStream *configStream
}

func newRemoteAgentDetails(registration *remoteagentregistry.RegistrationData, config config.Component) (*remoteAgentDetails, error) {
	client, err := newRemoteAgentClient(registration)
	if err != nil {
		return nil, err
	}

	rad := &remoteAgentDetails{
		displayName: registration.DisplayName,
		apiEndpoint: registration.APIEndpoint,
		client:      client,
		lastSeen:    time.Now(),
	}

	if err := rad.startConfigStream(config); err != nil {
		return nil, err
	}

	return rad, nil
}

func (rad *remoteAgentDetails) startConfigStream(config config.Component) error {
	if rad.configStream != nil {
		return errors.New("config stream already started")
	}

	ctx, ctxCancel := context.WithCancel(context.Background())
	configUpdates := make(chan *pb.ConfigUpdate, 8)

	rad.configStream = &configStream{
		ctxCancel:     ctxCancel,
		configUpdates: configUpdates,
	}

	// Create a new config updates stream over gRPC.
	stream, err := rad.client.StreamConfigEvents(ctx)
	if err != nil {
		rad.configStream = nil // We failed to start the stream, so reset the config stream state.
		return err
	}

	// Spawn a goroutine to drain incoming config updates and send them to the remote agent.
	go runConfigStream(ctx, config, stream, configUpdates)

	return nil
}
