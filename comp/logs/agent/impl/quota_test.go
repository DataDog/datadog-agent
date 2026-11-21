// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/quota"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

var validQuota = fmt.Sprintf(`{"quotas":[{"tags":["team:a"],"expires_at":%d}]}`, time.Now().Add(time.Hour).Unix())

func newTestQuotaManager(t *testing.T, product string) *quotaManager {
	return &quotaManager{log: logmock.New(t), enforcer: quota.NewEnforcer(), product: product}
}

func TestQuotaRCApplyState(t *testing.T) {
	m := newTestQuotaManager(t, state.ProductLogsQuota)
	got := map[string]state.ApplyStatus{}
	m.onRCUpdate(map[string]state.RawConfig{
		"datadog/2/LOGS_QUOTA/q1/config": {Config: []byte(validQuota)},
		"datadog/2/LOGS_QUOTA/q2/config": {Config: []byte(`{"hello":"world"}`)},
	}, func(path string, s state.ApplyStatus) { got[path] = s })

	assert.Equal(t, state.ApplyStateAcknowledged, got["datadog/2/LOGS_QUOTA/q1/config"].State)
	assert.Equal(t, state.ApplyStateError, got["datadog/2/LOGS_QUOTA/q2/config"].State)
	assert.Contains(t, got["datadog/2/LOGS_QUOTA/q2/config"].Error, "missing quotas")
}

func TestQuotaRCDebugProduct(t *testing.T) {
	m := newTestQuotaManager(t, state.ProductDebug)
	got := map[string]state.ApplyStatus{}
	m.onRCUpdate(map[string]state.RawConfig{
		"datadog/2/DEBUG/ours/config":   {Config: []byte(validQuota)},
		"datadog/2/DEBUG/theirs/config": {Config: []byte(`{"hello":"world"}`)},
	}, func(path string, s state.ApplyStatus) { got[path] = s })

	assert.Equal(t, map[string]state.ApplyStatus{
		"datadog/2/DEBUG/ours/config": {State: state.ApplyStateAcknowledged},
	}, got)
	_, configs := m.enforcer.Status()
	require.Len(t, configs, 1)
	require.Len(t, configs[0].Quotas, 1)
	assert.Equal(t, "enforcing", configs[0].Quotas[0].State)
}
