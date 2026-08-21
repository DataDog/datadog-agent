// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentimpl

import (
	"context"
	"errors"
	"testing"
	"time"

	parsigningkeys "github.com/DataDog/datadog-agent/comp/privateactionrunner/signingkeys/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/signingkeys"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakePARSigningKeys struct {
	snapshot parsigningkeys.Snapshot
	err      error
}

func (f fakePARSigningKeys) Get(knownRevision uint64) (parsigningkeys.Snapshot, error) {
	f.snapshot.Unchanged = f.snapshot.Initialized && knownRevision == f.snapshot.Revision
	return f.snapshot, f.err
}

func TestGetPARSigningKeys(t *testing.T) {
	updatedAt := time.Now().UTC()
	server := &serverSecure{parSigningKeys: fakePARSigningKeys{snapshot: parsigningkeys.Snapshot{
		Keys:        []signingkeys.Key{{ID: "key", KeyType: types.KeyTypeED25519, Key: []byte("pem")}},
		Revision:    4,
		UpdatedAt:   updatedAt,
		Initialized: true,
	}}}

	response, err := server.GetPARSigningKeys(context.Background(), &pb.GetPARSigningKeysRequest{})
	require.NoError(t, err)
	require.Equal(t, uint64(4), response.GetRevision())
	require.Equal(t, updatedAt, response.GetUpdatedAt().AsTime())
	require.Equal(t, []*pb.PARSigningKey{{Id: "key", KeyType: "ED25519", Key: []byte("pem")}}, response.GetKeys())

	response, err = server.GetPARSigningKeys(context.Background(), &pb.GetPARSigningKeysRequest{KnownRevision: 4})
	require.NoError(t, err)
	require.True(t, response.GetUnchanged())
	require.Empty(t, response.GetKeys())
}

func TestGetPARSigningKeysUnavailable(t *testing.T) {
	server := &serverSecure{parSigningKeys: fakePARSigningKeys{err: errors.New("not ready")}}
	_, err := server.GetPARSigningKeys(context.Background(), &pb.GetPARSigningKeysRequest{})
	require.Equal(t, codes.Unavailable, status.Code(err))
}
