// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"context"
	"math/rand/v2"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/signingkeys"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func signingKeyPollDelay() time.Duration {
	// Poll between four and five seconds to avoid synchronized executor traffic
	// without exceeding the five-second propagation target.
	return 4*time.Second + time.Duration(rand.Int64N(int64(time.Second)))
}

// StartSigningKeySync starts synchronization from the Core Agent. Initial
// synchronization counts as activity; periodic refreshes do not.
func (s *Server) StartSigningKeySync(ctx context.Context, client SigningKeysClient) {
	if s.keysManager == nil {
		return
	}
	if s.keysManager.IsReady() {
		s.SetReady(true)
		return
	}
	go s.syncSigningKeys(ctx, client, signingKeyPollDelay)
}

func (s *Server) syncSigningKeys(ctx context.Context, client SigningKeysClient, nextDelay func() time.Duration) {
	initialSyncStarted := time.Now()
	finishInitialSync := s.beginActivity()
	initialSyncPending := true
	defer func() {
		if initialSyncPending {
			finishInitialSync()
		}
	}()

	var revision uint64
	for {
		response, err := client.GetPARSigningKeys(ctx, &pb.GetPARSigningKeysRequest{KnownRevision: revision})
		if err != nil {
			log.FromContext(ctx).Warn("Could not synchronize signing keys from the Core Agent", log.ErrorField(err))
		} else {
			if s.applySigningKeyResponse(ctx, response) {
				revision = response.GetRevision()
			}
			if s.keysManager.IsReady() && initialSyncPending {
				finishInitialSync()
				initialSyncPending = false
				log.FromContext(ctx).Info(
					"Initial signing-key synchronization completed",
					log.Duration("duration", time.Since(initialSyncStarted)),
				)
			}
		}

		timer := time.NewTimer(nextDelay())
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
}

func (s *Server) applySigningKeyResponse(ctx context.Context, response *pb.GetPARSigningKeysResponse) bool {
	if !response.GetInitialized() {
		s.keysManager.MarkExpired()
		s.SetReady(false)
		return true
	}
	if response.GetConfigStatus() == pb.ConfigStatus_CONFIG_STATUS_EXPIRED {
		s.keysManager.MarkExpired()
		s.SetReady(false)
		return true
	}
	if response.GetUnchanged() {
		s.SetReady(s.keysManager.IsReady())
		return true
	}

	keys := make([]signingkeys.Key, 0, len(response.GetKeys()))
	for _, key := range response.GetKeys() {
		keys = append(keys, signingkeys.Key{
			ID:      key.GetId(),
			KeyType: types.KeyType(key.GetKeyType()),
			Key:     append([]byte(nil), key.GetKey()...),
		})
	}
	if err := s.keysManager.InstallAuthoritative(keys); err != nil {
		log.FromContext(ctx).Error("Core Agent returned invalid signing keys", log.ErrorField(err))
		return false
	}
	s.SetReady(true)
	return true
}
