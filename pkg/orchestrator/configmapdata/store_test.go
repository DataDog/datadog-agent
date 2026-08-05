// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configmapdata

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// rawConfig wraps a JSON payload the way remote config delivers it.
func rawConfig(contents string) state.RawConfig {
	return state.RawConfig{Config: []byte(contents)}
}

// applyRecorder collects the apply statuses reported back to remote config.
type applyRecorder struct {
	statuses map[string]state.ApplyStatus
}

func newApplyRecorder() *applyRecorder {
	return &applyRecorder{statuses: make(map[string]state.ApplyStatus)}
}

func (r *applyRecorder) callback(path string, status state.ApplyStatus) {
	r.statuses[path] = status
}

func TestStoreSnapshotFiltersByCluster(t *testing.T) {
	s := &Store{}
	s.Replace([]Entry{
		{ClusterID: "cluster-a", Namespace: "default", Name: "cm-1", Timestamp: 1, DataCollected: true},
		{ClusterID: "cluster-b", Namespace: "default", Name: "cm-1", Timestamp: 1, DataCollected: true},
	})

	a := s.Snapshot("cluster-a")
	assert.True(t, a.IsAllowed("default", "cm-1"))
	assert.Len(t, a, 1)

	// Same namespace and name in another cluster must not match: remote config configs are
	// org-scoped, so kube-system/coredns exists in every cluster of the org.
	assert.False(t, a.IsAllowed("kube-system", "cm-1"))
	assert.False(t, s.Snapshot("cluster-c").IsAllowed("default", "cm-1"))
}

func TestStoreSnapshotIsStable(t *testing.T) {
	s := &Store{}
	s.Replace([]Entry{{ClusterID: "cluster-a", Namespace: "default", Name: "cm-1", Timestamp: 1, DataCollected: true}})

	snapshot := s.Snapshot("cluster-a")
	s.Replace(nil)

	// The snapshot taken at the start of a tick keeps reporting the old answer, which is what makes
	// the resource version and the strip decision agree within a tick.
	assert.True(t, snapshot.IsAllowed("default", "cm-1"))
	assert.False(t, s.Snapshot("cluster-a").IsAllowed("default", "cm-1"))
}

func TestSnapshotLookupSeparatesOptedOutFromUnknown(t *testing.T) {
	s := &Store{}
	s.Replace([]Entry{{ClusterID: "cluster-a", Namespace: "default", Name: "cm-1", Timestamp: 200}})

	snapshot := s.Snapshot("cluster-a")

	// Both are stripped, but only one of them has a decision behind it, and the two have to report
	// different resource versions for the opt-out to reach storage.
	assert.False(t, snapshot.IsAllowed("default", "cm-1"))
	assert.False(t, snapshot.IsAllowed("default", "cm-unknown"))

	optedOut, mentioned := snapshot.Lookup("default", "cm-1")
	assert.True(t, mentioned)
	assert.Equal(t, int64(200), optedOut.Timestamp)

	_, mentioned = snapshot.Lookup("default", "cm-unknown")
	assert.False(t, mentioned)
}

func TestStoreEmptySnapshot(t *testing.T) {
	s := &Store{}
	assert.False(t, s.Snapshot("cluster-a").IsAllowed("default", "cm-1"))
	assert.Zero(t, s.Len())
}

func TestOnUpdate(t *testing.T) {
	tests := []struct {
		name          string
		update        map[string]state.RawConfig
		expected      []Entry
		expectedState map[string]state.ApplyState
	}{
		{
			name: "single config",
			update: map[string]state.RawConfig{
				"datadog/2/DEBUG/cm/config": rawConfig(`{"version":1,"configmaps":[{"cluster_id":"cluster-a","namespace":"default","name":"cm-1","timestamp":100,"data_collected":true}]}`),
			},
			expected:      []Entry{{ClusterID: "cluster-a", Namespace: "default", Name: "cm-1", Timestamp: 100, DataCollected: true}},
			expectedState: map[string]state.ApplyState{"datadog/2/DEBUG/cm/config": state.ApplyStateAcknowledged},
		},
		{
			// An opt-out is an entry, not a removal, so that the reported resource version keeps
			// moving and the write that strips the data is not deduped away by the backend.
			name: "an opt-out is kept as an entry",
			update: map[string]state.RawConfig{
				"datadog/2/DEBUG/cm/config": rawConfig(`{"configmaps":[{"cluster_id":"cluster-a","namespace":"default","name":"cm-1","timestamp":200,"data_collected":false}]}`),
			},
			expected:      []Entry{{ClusterID: "cluster-a", Namespace: "default", Name: "cm-1", Timestamp: 200}},
			expectedState: map[string]state.ApplyState{"datadog/2/DEBUG/cm/config": state.ApplyStateAcknowledged},
		},
		{
			name: "union across configs, duplicates collapsed",
			update: map[string]state.RawConfig{
				"datadog/2/DEBUG/cm-a/config": rawConfig(`{"configmaps":[{"cluster_id":"cluster-a","namespace":"default","name":"cm-1","timestamp":100,"data_collected":true}]}`),
				"datadog/2/DEBUG/cm-b/config": rawConfig(`{"configmaps":[{"cluster_id":"cluster-a","namespace":"default","name":"cm-1","timestamp":100,"data_collected":true},{"cluster_id":"cluster-a","namespace":"kube-system","name":"cm-2","timestamp":100,"data_collected":true}]}`),
			},
			expected: []Entry{
				{ClusterID: "cluster-a", Namespace: "default", Name: "cm-1", Timestamp: 100, DataCollected: true},
				{ClusterID: "cluster-a", Namespace: "kube-system", Name: "cm-2", Timestamp: 100, DataCollected: true},
			},
			expectedState: map[string]state.ApplyState{
				"datadog/2/DEBUG/cm-a/config": state.ApplyStateAcknowledged,
				"datadog/2/DEBUG/cm-b/config": state.ApplyStateAcknowledged,
			},
		},
		{
			// Two decisions for the same ConfigMap resolve to the newest one. Configs are walked in
			// sorted path order, so without this the winner would be whichever config happened to
			// come last, and a stale opt-in could undo an opt-out.
			name: "conflicting decisions resolve to the newest timestamp",
			update: map[string]state.RawConfig{
				"datadog/2/DEBUG/cm-a/config": rawConfig(`{"configmaps":[{"cluster_id":"cluster-a","namespace":"default","name":"cm-1","timestamp":300,"data_collected":false}]}`),
				"datadog/2/DEBUG/cm-b/config": rawConfig(`{"configmaps":[{"cluster_id":"cluster-a","namespace":"default","name":"cm-1","timestamp":100,"data_collected":true}]}`),
			},
			expected: []Entry{{ClusterID: "cluster-a", Namespace: "default", Name: "cm-1", Timestamp: 300}},
			expectedState: map[string]state.ApplyState{
				"datadog/2/DEBUG/cm-a/config": state.ApplyStateAcknowledged,
				"datadog/2/DEBUG/cm-b/config": state.ApplyStateAcknowledged,
			},
		},
		{
			// An entry without a timestamp would report the same resource version whatever its state,
			// so it could never propagate a change.
			name: "incomplete entries are ignored",
			update: map[string]state.RawConfig{
				"datadog/2/DEBUG/cm/config": rawConfig(`{"configmaps":[{"cluster_id":"cluster-a","namespace":"default","timestamp":100},{"namespace":"default","name":"cm-1","timestamp":100},{"cluster_id":"cluster-a","namespace":"default","name":"cm-3","data_collected":true},{"cluster_id":"cluster-a","namespace":"default","name":"cm-2","timestamp":100,"data_collected":true}]}`),
			},
			expected:      []Entry{{ClusterID: "cluster-a", Namespace: "default", Name: "cm-2", Timestamp: 100, DataCollected: true}},
			expectedState: map[string]state.ApplyState{"datadog/2/DEBUG/cm/config": state.ApplyStateAcknowledged},
		},
		{
			name: "malformed config is reported as an error",
			update: map[string]state.RawConfig{
				"datadog/2/DEBUG/cm/config": rawConfig(`{"configmaps":`),
			},
			expected:      nil,
			expectedState: map[string]state.ApplyState{"datadog/2/DEBUG/cm/config": state.ApplyStateError},
		},
		{
			name: "a malformed config does not discard a valid one",
			update: map[string]state.RawConfig{
				"datadog/2/DEBUG/cm-a/config": rawConfig(`not json`),
				"datadog/2/DEBUG/cm-b/config": rawConfig(`{"configmaps":[{"cluster_id":"cluster-a","namespace":"default","name":"cm-1","timestamp":100,"data_collected":true}]}`),
			},
			expected: []Entry{{ClusterID: "cluster-a", Namespace: "default", Name: "cm-1", Timestamp: 100, DataCollected: true}},
			expectedState: map[string]state.ApplyState{
				"datadog/2/DEBUG/cm-a/config": state.ApplyStateError,
				"datadog/2/DEBUG/cm-b/config": state.ApplyStateAcknowledged,
			},
		},
		{
			// Remote config sends the full set every time, so no config left means nothing is opted
			// in. This is the opt-out path.
			name:          "empty update empties the store",
			update:        map[string]state.RawConfig{},
			expected:      nil,
			expectedState: map[string]state.ApplyState{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Get().Replace([]Entry{{ClusterID: "stale", Namespace: "stale", Name: "stale", Timestamp: 1}})
			t.Cleanup(func() { Get().Replace(nil) })

			recorder := newApplyRecorder()
			onUpdate(tt.update, recorder.callback)

			assert.ElementsMatch(t, tt.expected, Get().entries)

			require.Len(t, recorder.statuses, len(tt.expectedState))
			for path, expectedState := range tt.expectedState {
				assert.Equal(t, expectedState, recorder.statuses[path].State, "apply state for %s", path)
			}
		})
	}
}

func TestOnUpdateCapsEntries(t *testing.T) {
	t.Cleanup(func() { Get().Replace(nil) })

	entries := make([]string, 0, maxEntries+50)
	for i := range maxEntries + 50 {
		entries = append(entries, fmt.Sprintf(`{"cluster_id":"cluster-a","namespace":"default","name":"cm-%d","timestamp":100,"data_collected":true}`, i))
	}

	recorder := newApplyRecorder()
	onUpdate(map[string]state.RawConfig{
		"datadog/2/DEBUG/cm/config": rawConfig(`{"configmaps":[` + strings.Join(entries, ",") + `]}`),
	}, recorder.callback)

	assert.Equal(t, maxEntries, Get().Len())
}

func TestStoreConcurrentAccess(t *testing.T) {
	s := &Store{}

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.Replace([]Entry{{ClusterID: "cluster-a", Namespace: "default", Name: fmt.Sprintf("cm-%d", i), Timestamp: 1, DataCollected: true}})
		}()
		go func() {
			defer wg.Done()
			s.Snapshot("cluster-a").IsAllowed("default", "cm-1")
			s.Len()
		}()
	}
	wg.Wait()

	// Every Replace stores exactly one entry, so whichever goroutine wrote last, the store holds one.
	assert.Equal(t, 1, s.Len())
}

func TestSubscribeUsesTheDebugProduct(t *testing.T) {
	var product string
	Subscribe(func(p string, _ func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
		product = p
	})
	assert.Equal(t, state.ProductDebug, product)
}
