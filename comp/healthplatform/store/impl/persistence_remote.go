// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package storeimpl

import (
	"context"
	"time"

	remotedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/remote/def"
)

// remotePersistence adapts the remote issue loader to the store's shared
// persistence path. Writes are handled by Health Platform egress.
type remotePersistence struct {
	loader remotedef.Component
}

var _ issuesPersistence = (*remotePersistence)(nil)

func newRemotePersistence(loader remotedef.Component) *remotePersistence {
	return &remotePersistence{loader: loader}
}

func (r *remotePersistence) load(ctx context.Context) (*PersistedState, error) {
	issues, err := r.loader.Load(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().Format(time.RFC3339)
	state := &PersistedState{
		Version:   persistedStateVersion,
		UpdatedAt: now,
		Issues:    make(map[string]*PersistedIssue, len(issues)),
	}
	for _, issue := range issues {
		state.Issues[issue.IssueID] = &PersistedIssue{
			IssueID:        issue.IssueID,
			IssueType:      issue.IssueName,
			ProtoIssueType: issue.IssueType,
			Category:       issue.Category,
			State:          IssueStateActive,
			FirstSeen:      now,
			LastSeen:       now,
		}
	}
	return state, nil
}

func (*remotePersistence) save(*PersistedState) error { return nil }
