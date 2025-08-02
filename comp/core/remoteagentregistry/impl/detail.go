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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type remoteAgentDetails struct {
	lastSeen             time.Time
	displayName          string
	sanatizedDisplayName string
	apiEndpoint          string
	client               pb.RemoteAgentClient
	configStream         *configStream
}

func newRemoteAgentDetails(registration *remoteagentregistry.RegistrationData, config config.Component) (*remoteAgentDetails, error) {
	client, err := newRemoteAgentClient(registration)
	if err != nil {
		return nil, err
	}

	rad := &remoteAgentDetails{
		displayName:          registration.DisplayName,
		apiEndpoint:          registration.APIEndpoint,
		sanatizedDisplayName: sanatizeString(registration.DisplayName),
		client:               client,
		lastSeen:             time.Now(),
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

type configStream struct {
	ctxCancel     context.CancelFunc
	configUpdates chan *pb.ConfigUpdate
}

func (cs *configStream) Cancel() {
	cs.ctxCancel()
}

func (cs *configStream) TrySendUpdate(update *pb.ConfigUpdate) bool {
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()

	select {
	case cs.configUpdates <- update:
		return true
	case <-timer.C:
		return false
	}
}

func runConfigStream(ctx context.Context, config config.Component, stream pb.RemoteAgent_StreamConfigEventsClient, configUpdates chan *pb.ConfigUpdate) {
	retryInterval := config.GetDuration("remote_agent_registry.config_stream_retry_interval")
	var cachedSnapshot *pb.ConfigEvent
	var lastEventSequenceID uint64

outer:
	for {

		// Start by sending an initial snapshot of the current configuration.
		//
		// We do this to ensure that when we restart this outer loop, we always resynchronize the remote agent by
		// providing a complete snapshot of the current configuration. This lets us handle any errors during send
		// by just restarting the outer loop.
		if cachedSnapshot == nil || lastEventSequenceID != config.GetSequenceID() {
			initialSnapshot, sequenceID, err := createConfigSnapshot(config)

			if err != nil {
				log.Errorf("Failed to create initial config snapshot: %v", err)
				time.Sleep(retryInterval)
				continue
			}

			cachedSnapshot = initialSnapshot
			lastEventSequenceID = sequenceID
		}

		// Always send the (possibly cached) snapshot on a new connection.
		err := stream.Send(cachedSnapshot)
		if err != nil {
			log.Errorf("Failed to send initial config snapshot to remote agent: %v", err)
			time.Sleep(retryInterval)
			continue
		}

		// Start processing config updates.
		for {
			select {
			case <-ctx.Done():
				return
			case configUpdate := <-configUpdates:
				// If this update is older than the last event we sent, ignore it.
				//
				// If the sequence ID doesn't immediately follow our last event's sequence ID, then we've out of sync and
				// need to restart the outer loop to resynchronize.
				currentSequenceID := uint64(configUpdate.SequenceId)
				if currentSequenceID <= lastEventSequenceID {
					continue
				}

				if currentSequenceID > lastEventSequenceID+1 {
					continue outer
				}

				update := &pb.ConfigEvent{
					Event: &pb.ConfigEvent_Update{
						Update: configUpdate,
					},
				}

				err := stream.Send(update)
				if err != nil {
					log.Errorf("Failed to send config update to remote agent: %v", err)
					continue outer
				}

				lastEventSequenceID = currentSequenceID
			}
		}
	}
}
