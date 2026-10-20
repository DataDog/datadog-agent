// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package storeimpl

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	remotedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/remote/def"
)

type fakeRemoteIssueLoader struct {
	issues []remotedef.Issue
	err    error
	calls  atomic.Int32
}

func (l *fakeRemoteIssueLoader) Load(context.Context) ([]remotedef.Issue, error) {
	l.calls.Add(1)
	return l.issues, l.err
}

func TestLoadFromRemoteRestoresReconciliationState(t *testing.T) {
	const issueID = "invalid-config:daemonset-uid"
	loader := &fakeRemoteIssueLoader{issues: []remotedef.Issue{{
		IssueID:   issueID,
		IssueName: "Invalid Agent Configuration",
		IssueType: "invalid_agent_configuration",
		Category:  "configuration",
	}}}

	h := newTestStore(t)
	h.persistence = newRemotePersistence(loader)
	resolvedCh := make(chan *healthplatformpayload.Issue, 1)
	h.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: resolvedCh})

	require.NoError(t, h.start(context.Background()))
	assert.Equal(t, []string{issueID}, h.GetActiveIssueIDsByIssueName("Invalid Agent Configuration"))

	persisted := h.persistedIssues[issueID]
	require.NotNil(t, persisted)
	assert.Equal(t, "Invalid Agent Configuration", persisted.IssueType)
	assert.Equal(t, "invalid_agent_configuration", persisted.ProtoIssueType)
	assert.Equal(t, "configuration", persisted.Category)
	assert.Equal(t, IssueStateActive, persisted.State)
	assert.NotEmpty(t, persisted.FirstSeen)
	assert.NotEmpty(t, persisted.LastSeen)

	h.ResolveIssue(issueID)
	require.Len(t, resolvedCh, 1)
	resolved := <-resolvedCh
	assert.Equal(t, issueID, resolved.Id)
	assert.Equal(t, "Invalid Agent Configuration", resolved.IssueName)
	assert.Equal(t, "invalid_agent_configuration", resolved.IssueType)
	assert.Equal(t, "configuration", resolved.Category)
	require.NotNil(t, resolved.PersistedIssue)
	assert.Equal(t, IssueStateResolved, resolved.PersistedIssue.State)
	assert.Equal(t, int32(1), loader.calls.Load())
}

func TestLoadFromRemoteMatchesDiskIssueProcessing(t *testing.T) {
	remote := newTestStore(t)
	remote.persistence = newRemotePersistence(&fakeRemoteIssueLoader{issues: []remotedef.Issue{
		{IssueID: "duplicate", IssueName: "Old", IssueType: "old", Category: "configuration"},
		{IssueID: "duplicate", IssueName: "New"},
		{IssueName: "Empty ID", IssueType: "empty_id", Category: "configuration"},
		{IssueID: "empty-name", IssueType: "empty_name", Category: "configuration"},
		{IssueID: "empty-type", IssueName: "Empty Type", Category: "configuration"},
		{IssueID: "empty-category", IssueName: "Empty Category", IssueType: "empty_category"},
	}})
	require.NoError(t, remote.loadPersistedState(context.Background()))

	path := filepath.Join(t.TempDir(), "issues.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":2,"issues":{
		"duplicate":{"issue_type":"Old","proto_issue_type":"old","category":"configuration","state":"active"},
		"duplicate":{"issue_type":"New","state":"active"},
		"":{"issue_type":"Empty ID","proto_issue_type":"empty_id","category":"configuration","state":"active"},
		"empty-name":{"proto_issue_type":"empty_name","category":"configuration","state":"active"},
		"empty-type":{"issue_type":"Empty Type","category":"configuration","state":"active"},
		"empty-category":{"issue_type":"Empty Category","proto_issue_type":"empty_category","state":"active"}
	}}`), 0600))
	disk := newTestStore(t)
	disk.persistence = newDiskPersistence(path, disk.log)
	require.NoError(t, disk.loadPersistedState(context.Background()))

	require.Len(t, remote.persistedIssues, 5)
	assert.Equal(t, []string{"duplicate"}, remote.GetActiveIssueIDsByIssueName("New"))
	assert.Equal(t, disk.issuesByName, remote.issuesByName)
	require.Len(t, disk.persistedIssues, len(remote.persistedIssues))
	for id, want := range disk.persistedIssues {
		got := remote.persistedIssues[id]
		require.NotNil(t, got, "issue %q", id)
		assert.Equal(t, want.IssueID, got.IssueID)
		assert.Equal(t, want.IssueType, got.IssueType)
		assert.Equal(t, want.ProtoIssueType, got.ProtoIssueType)
		assert.Equal(t, want.Category, got.Category)
		assert.Equal(t, want.State, got.State)
	}
}

func TestRemotePersistencePropagatesLoadError(t *testing.T) {
	h := newTestStore(t)
	h.persistence = newRemotePersistence(&fakeRemoteIssueLoader{err: assert.AnError})
	assert.ErrorIs(t, h.loadPersistedState(context.Background()), assert.AnError)
}
