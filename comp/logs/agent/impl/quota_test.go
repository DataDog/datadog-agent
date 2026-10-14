// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/quota"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const validQuota = `{"schema_version":1,"quota_id":"q1","version":1,"action":"drop",
	"match":{"all":[{"field":"source","op":"eq","value":"nginx"}]},"max_ttl_seconds":60}`

func newTestQuotaManager(t *testing.T) *quotaManager {
	return &quotaManager{log: logmock.New(t), enforcer: quota.NewEnforcer()}
}

func TestQuotaRCApplyState(t *testing.T) {
	m := newTestQuotaManager(t)
	got := map[string]state.ApplyStatus{}
	m.onRCUpdate(map[string]state.RawConfig{
		"datadog/2/LOGS_QUOTA/q1/config": {Config: []byte(validQuota)},
		"datadog/2/LOGS_QUOTA/q2/config": {Config: []byte(`{"schema_version":9}`)},
	}, func(path string, s state.ApplyStatus) { got[path] = s })

	assert.Equal(t, state.ApplyStateAcknowledged, got["datadog/2/LOGS_QUOTA/q1/config"].State)
	assert.Equal(t, state.ApplyStateError, got["datadog/2/LOGS_QUOTA/q2/config"].State)
	assert.Contains(t, got["datadog/2/LOGS_QUOTA/q2/config"].Error, "schema_version")
}

func TestQuotaMockDir(t *testing.T) {
	m := newTestQuotaManager(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "q1.json"), []byte(validQuota), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("x"), 0o644))

	last := m.pollMockDir(dir, nil)
	_, quotas := m.enforcer.Status()
	require.Len(t, quotas, 1)
	assert.Equal(t, "mock", quotas[0].Source)
	assert.Equal(t, "q1.json", quotas[0].Path)
	assert.Equal(t, "enforcing", quotas[0].State)

	require.NoError(t, os.Remove(filepath.Join(dir, "q1.json")))
	m.pollMockDir(dir, last)
	_, quotas = m.enforcer.Status()
	assert.Empty(t, quotas)
}
