// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"context"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type signingKeysResult struct {
	response *pb.GetPARSigningKeysResponse
	err      error
}

type scriptedSigningKeysClient struct {
	requests  chan *pb.GetPARSigningKeysRequest
	responses chan signingKeysResult
}

func (c *scriptedSigningKeysClient) GetPARSigningKeys(ctx context.Context, request *pb.GetPARSigningKeysRequest) (*pb.GetPARSigningKeysResponse, error) {
	select {
	case c.requests <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case result := <-c.responses:
		return result.response, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestInitialSigningKeySyncIsActivityButPeriodicPollIsNot(t *testing.T) {
	manager := taskverifier.NewExecutorKeyManager()
	srv := NewServer(&fakeExecutor{}, "test", manager)
	mockClock := clock.NewMock()
	srv.clock = mockClock
	srv.touch()
	client := &scriptedSigningKeysClient{
		requests:  make(chan *pb.GetPARSigningKeysRequest),
		responses: make(chan signingKeysResult),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.syncSigningKeys(ctx, client, func() time.Duration { return time.Millisecond })
	}()

	request := <-client.requests
	require.Zero(t, request.GetKnownRevision())
	mockClock.Add(2 * time.Minute)
	assert.Zero(t, srv.idleFor())

	client.responses <- signingKeysResult{response: &pb.GetPARSigningKeysResponse{
		Revision: 1, Initialized: true, ConfigStatus: pb.ConfigStatus_CONFIG_STATUS_OK,
	}}
	request = <-client.requests
	require.Equal(t, uint64(1), request.GetKnownRevision())
	client.responses <- signingKeysResult{response: &pb.GetPARSigningKeysResponse{
		Revision: 1, Initialized: true, ConfigStatus: pb.ConfigStatus_CONFIG_STATUS_OK, Unchanged: true,
	}}

	require.Eventually(t, func() bool { return manager.IsReady() }, time.Second, time.Millisecond)
	mockClock.Add(time.Minute)
	assert.Equal(t, time.Minute, srv.idleFor(), "periodic polling must not count as activity")

	cancel()
	<-done
}

func TestSigningKeyExpirationAndRecovery(t *testing.T) {
	manager := taskverifier.NewExecutorKeyManager()
	srv := NewServer(&fakeExecutor{}, "test", manager)

	srv.applySigningKeyResponse(context.Background(), &pb.GetPARSigningKeysResponse{
		Revision: 1, Initialized: true, ConfigStatus: pb.ConfigStatus_CONFIG_STATUS_OK,
	})
	require.True(t, manager.IsReady())

	srv.applySigningKeyResponse(context.Background(), &pb.GetPARSigningKeysResponse{
		Revision: 2, Initialized: true, ConfigStatus: pb.ConfigStatus_CONFIG_STATUS_EXPIRED,
	})
	require.False(t, manager.IsReady())
	health, err := srv.Health(context.Background(), nil)
	require.NoError(t, err)
	require.False(t, health.GetReady())

	srv.applySigningKeyResponse(context.Background(), &pb.GetPARSigningKeysResponse{
		Revision: 3, Initialized: true, ConfigStatus: pb.ConfigStatus_CONFIG_STATUS_OK,
	})
	require.True(t, manager.IsReady())
	health, err = srv.Health(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, health.GetReady())
}

func TestMalformedSigningKeyUpdatePreservesReadiness(t *testing.T) {
	manager := taskverifier.NewExecutorKeyManager()
	srv := NewServer(&fakeExecutor{}, "test", manager)
	srv.applySigningKeyResponse(context.Background(), &pb.GetPARSigningKeysResponse{
		Revision: 1, Initialized: true, ConfigStatus: pb.ConfigStatus_CONFIG_STATUS_OK,
	})

	srv.applySigningKeyResponse(context.Background(), &pb.GetPARSigningKeysResponse{
		Revision:     2,
		Initialized:  true,
		ConfigStatus: pb.ConfigStatus_CONFIG_STATUS_OK,
		Keys:         []*pb.PARSigningKey{{Id: "bad", KeyType: "ED25519", Key: []byte("bad")}},
	})

	require.True(t, manager.IsReady())
	health, err := srv.Health(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, health.GetReady())
}
