// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

var errTUFSignatureFailure = errors.New("signature verification failed")

// dcLatchedVersions models the TUF state of an agent latched to a DC that
// performed its first root rotation (root 18, from the embedded root 17).
var dcLatchedVersions = uptane.TUFVersions{
	ConfigRoot:      18,
	ConfigSnapshot:  1001,
	DirectorRoot:    18,
	DirectorTargets: 201,
}

var dcEmbeddedVersions = uptane.TUFVersions{
	ConfigRoot:   17,
	DirectorRoot: 17,
}

func TestRefreshDCSwitchRecovery(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)
	// The agent is already latched to a DC (not a first update).
	service.mu.firstUpdate = false

	normalResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}
	freshJoinResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "fresh-join"}},
	}

	// The poll reporting the old DC state.
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: dcLatchedVersions.ConfigSnapshot,
		CurrentConfigRootVersion:     dcLatchedVersions.ConfigRoot,
		CurrentDirectorRootVersion:   dcLatchedVersions.DirectorRoot,
		Products:                     []string{},
		NewProducts:                  []string{},
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
	}).Return(normalResponse, nil)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(dcLatchedVersions, nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)

	// The TUF update fails: the org moved to another DC behind the same URL,
	// the response is signed with the new DC's keys.
	uptaneClient.On("Update", normalResponse).Return(errTUFSignatureFailure)

	// The recovery re-requests a fresh-join payload: the embedded root
	// versions, everything else zeroed, and the TUF error reported to the
	// backend for observability.
	uptaneClient.On("EmbeddedRootVersions").Return(dcEmbeddedVersions, nil)
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                   service.hostname,
		TraceAgentEnv:              testEnv,
		AgentUuid:                  "test-uuid",
		AgentVersion:               agentVersion,
		CurrentConfigRootVersion:   dcEmbeddedVersions.ConfigRoot,
		CurrentDirectorRootVersion: dcEmbeddedVersions.DirectorRoot,
		Products:                   []string{},
		NewProducts:                []string{},
		HasError:                   true,
		Error:                      "tuf: signature verification failed",
		OrgUuid:                    "abcdef",
		Tags:                       getHostTags(),
	}).Return(freshJoinResponse, nil)

	uptaneClient.On("TryRecoverFromEmbeddedRoot", freshJoinResponse).Return(nil)

	// The recovery succeeded: the refresh completes normally.
	err := service.refresh()
	require.NoError(t, err)

	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)
}

func TestRefreshDCSwitchRecoveryCooldown(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)
	// The agent is already latched to a DC (not a first update).
	service.mu.firstUpdate = false

	normalResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}

	// The first poll reports the old DC state (no error yet). It fails the
	// TUF update and attempts a recovery, which fails too: the error is kept
	// and reported to the backend on the next polls.
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: dcLatchedVersions.ConfigSnapshot,
		CurrentConfigRootVersion:     dcLatchedVersions.ConfigRoot,
		CurrentDirectorRootVersion:   dcLatchedVersions.DirectorRoot,
		Products:                     []string{},
		NewProducts:                  []string{},
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
	}).Return(normalResponse, nil).Once()
	// The next polls still report the old DC state, with the TUF error.
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: dcLatchedVersions.ConfigSnapshot,
		CurrentConfigRootVersion:     dcLatchedVersions.ConfigRoot,
		CurrentDirectorRootVersion:   dcLatchedVersions.DirectorRoot,
		Products:                     []string{},
		NewProducts:                  []string{},
		HasError:                     true,
		Error:                        "tuf: signature verification failed",
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
	}).Return(normalResponse, nil).Times(2)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(dcLatchedVersions, nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)
	uptaneClient.On("Update", normalResponse).Return(errTUFSignatureFailure).Times(3)

	// Only the first refresh attempts a recovery: the second one, within the
	// cooldown, must not re-request anything.
	uptaneClient.On("EmbeddedRootVersions").Return(dcEmbeddedVersions, nil).Once()
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                   service.hostname,
		TraceAgentEnv:              testEnv,
		AgentUuid:                  "test-uuid",
		AgentVersion:               agentVersion,
		CurrentConfigRootVersion:   dcEmbeddedVersions.ConfigRoot,
		CurrentDirectorRootVersion: dcEmbeddedVersions.DirectorRoot,
		Products:                   []string{},
		NewProducts:                []string{},
		HasError:                   true,
		Error:                      "tuf: signature verification failed",
		OrgUuid:                    "abcdef",
		Tags:                       getHostTags(),
	}).Return(normalResponse, nil).Once()
	uptaneClient.On("TryRecoverFromEmbeddedRoot", normalResponse).Return(errTUFSignatureFailure).Once()

	// First refresh: TUF failure, recovery attempted and failed.
	err := service.refresh()
	assert.Error(t, err)
	assert.Equal(t, errTUFSignatureFailure, err)

	// Advance the clock by only a second (well within the cooldown).
	clock.Add(1 * time.Second)

	// Second refresh: TUF failure again, but no new recovery attempt. The
	// once()-scoped recovery expectations above would panic if it re-requested
	// anything, so reaching this point without a panic is the assertion.
	err = service.refresh()
	assert.Error(t, err)
	assert.Equal(t, errTUFSignatureFailure, err)

	// After the cooldown expires, a recovery is attempted again.
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                   service.hostname,
		TraceAgentEnv:              testEnv,
		AgentUuid:                  "test-uuid",
		AgentVersion:               agentVersion,
		CurrentConfigRootVersion:   dcEmbeddedVersions.ConfigRoot,
		CurrentDirectorRootVersion: dcEmbeddedVersions.DirectorRoot,
		Products:                   []string{},
		NewProducts:                []string{},
		HasError:                   true,
		Error:                      "tuf: signature verification failed",
		OrgUuid:                    "abcdef",
		Tags:                       getHostTags(),
	}).Return(normalResponse, nil).Once()
	uptaneClient.On("EmbeddedRootVersions").Return(dcEmbeddedVersions, nil).Once()
	uptaneClient.On("TryRecoverFromEmbeddedRoot", normalResponse).Return(errTUFSignatureFailure).Once()

	clock.Add(dcSwitchRecoveryCooldown)
	err = service.refresh()
	assert.Error(t, err)
	assert.Equal(t, errTUFSignatureFailure, err)

	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)
}
