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
	"testing/synctest"
	"time"

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
	loadFn func(context.Context) ([]remotedef.Issue, error)
}

func (l *fakeRemoteIssueLoader) Load(ctx context.Context) ([]remotedef.Issue, error) {
	l.calls.Add(1)
	if l.loadFn != nil {
		return l.loadFn(ctx)
	}
	return l.issues, l.err
}

func TestRemoteRestoreRetriesWithCappedBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTestStore(t)
		loader := &fakeRemoteIssueLoader{}
		loader.loadFn = func(context.Context) ([]remotedef.Issue, error) {
			if loader.calls.Load() < 7 {
				return nil, assert.AnError
			}
			return []remotedef.Issue{{IssueID: "restored", IssueName: "Test Issue"}}, nil
		}
		h.persistence = newRemotePersistence(loader)
		startCtx, cancelStart := context.WithCancel(context.Background())
		start := time.Now()
		require.NoError(t, h.start(startCtx))
		cancelStart() // The Fx startup context must not govern background retries.
		t.Cleanup(func() { require.NoError(t, h.stop(context.Background())) })
		assert.Equal(t, start, time.Now(), "startup must not wait for retries")

		for i, delay := range []time.Duration{5, 10, 20, 40, 60, 60} {
			synctest.Wait()
			time.Sleep(delay*time.Second - time.Nanosecond)
			assert.Equal(t, int32(i+1), loader.calls.Load(), "attempt before backoff elapsed")
			time.Sleep(time.Nanosecond)
			synctest.Wait()
			assert.Equal(t, int32(i+2), loader.calls.Load())
		}
		assert.Equal(t, []string{"restored"}, h.GetActiveIssueIDsByIssueName("Test Issue"))
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		assert.Equal(t, int32(7), loader.calls.Load(), "successful restoration must stop retrying")
	})
}

func TestRemoteRestoreStopCancelsRetry(t *testing.T) {
	for _, inFlight := range []bool{false, true} {
		name := "backoff"
		if inFlight {
			name = "in-flight request"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newTestStore(t)
				loader := &fakeRemoteIssueLoader{}
				canceled := make(chan struct{})
				loader.loadFn = func(ctx context.Context) ([]remotedef.Issue, error) {
					if loader.calls.Load() > 1 {
						<-ctx.Done()
						close(canceled)
						return nil, ctx.Err()
					}
					return nil, assert.AnError
				}
				h.persistence = newRemotePersistence(loader)
				require.NoError(t, h.start(context.Background()))
				synctest.Wait()
				if inFlight {
					time.Sleep(5 * time.Second)
					synctest.Wait()
					require.Equal(t, int32(2), loader.calls.Load())
				}
				require.NoError(t, h.stop(context.Background()))
				if inFlight {
					select {
					case <-canceled:
					default:
						t.Fatal("stop did not cancel the remote request")
					}
				}
				callsAtStop := loader.calls.Load()
				time.Sleep(2 * time.Minute)
				synctest.Wait()
				assert.Equal(t, callsAtStop, loader.calls.Load())
			})
		})
	}
}

func TestDelayedRemoteRestoreReconcilesWithoutOverwritingLocalState(t *testing.T) {
	for _, checkBeforeRestore := range []bool{false, true} {
		name := "check after restoration"
		if checkBeforeRestore {
			name = "check before restoration"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newTestStore(t)
				loader := &fakeRemoteIssueLoader{}
				loader.loadFn = func(context.Context) ([]remotedef.Issue, error) {
					if loader.calls.Load() == 1 {
						return nil, assert.AnError
					}
					return []remotedef.Issue{
						{IssueID: "stale", IssueName: "Test Issue", IssueType: "test_issue", Category: "configuration"},
						{IssueID: "continuing", IssueName: "Test Issue"},
						{IssueID: "resolved-locally", IssueName: "Test Issue"},
						{IssueID: "not-checked-yet", IssueName: "Other Issue"},
					}, nil
				}
				h.persistence = newRemotePersistence(loader)
				resolvedCh := make(chan *healthplatformpayload.Issue, 4)
				h.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: resolvedCh})
				require.NoError(t, h.start(context.Background()))
				t.Cleanup(func() { require.NoError(t, h.stop(context.Background())) })

				for _, id := range []string{"continuing", "new", "independent", "resolved-locally"} {
					require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{
						Id: id, IssueName: "Test Issue", IssueType: "local_type", Category: "local_category",
					}))
				}
				h.ResolveIssue("resolved-locally")
				<-resolvedCh
				if checkBeforeRestore {
					h.ReconcilePersistedIssues([]string{"Test Issue"})
				}
				continuingBefore := h.GetIssue("continuing")

				synctest.Wait()
				time.Sleep(5 * time.Second)
				synctest.Wait()
				if !checkBeforeRestore {
					h.ReconcilePersistedIssues([]string{"Test Issue"})
				}
				assert.ElementsMatch(t, []string{"continuing", "new", "independent"}, h.GetActiveIssueIDsByIssueName("Test Issue"))
				assert.Equal(t, continuingBefore, h.GetIssue("continuing"))
				assert.Equal(t, IssueStateResolved, h.persistedIssues["resolved-locally"].State)
				assert.Equal(t, []string{"not-checked-yet"}, h.GetActiveIssueIDsByIssueName("Other Issue"))
				require.Len(t, resolvedCh, 1)
				resolved := <-resolvedCh
				assert.Equal(t, "stale", resolved.Id)
				assert.Equal(t, "test_issue", resolved.IssueType)
				assert.Equal(t, "configuration", resolved.Category)
				assert.Equal(t, IssueStateResolved, resolved.PersistedIssue.State)
			})
		})
	}
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
