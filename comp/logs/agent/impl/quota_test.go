// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/quota"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	logsStatus "github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

var validQuota = fmt.Sprintf(`{"quotas":[{"tags":["team:a"],"expires_at":%d}]}`, time.Now().Add(time.Hour).Unix())

func newTestQuotaManager(t *testing.T, product string) *quotaManager {
	hostname, _ := hostnamemock.NewMock("my-host")
	return &quotaManager{log: logmock.New(t), cfg: configmock.New(t), hostname: hostname, enforcer: quota.NewEnforcer(), product: product, clock: clock.NewMock()}
}

func quotaConfig(tags string) map[string]state.RawConfig {
	return map[string]state.RawConfig{"datadog/2/LOGS_QUOTA/q/config": {Config: fmt.Appendf(nil,
		`{"quotas":[{"tags":[%s],"expires_at":%d}]}`, tags, time.Now().Add(time.Hour).Unix())}}
}

func noopApply(string, state.ApplyStatus) {}

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

func TestQuotaHostTagsFromConfig(t *testing.T) {
	t.Setenv("DD_TAGS", "team:env-var")
	cfg := configmock.NewFromYAML(t, "tags: [team:yaml]\nextra_tags: [extra:x]\nenv: Prod\n")
	m := newTestQuotaManager(t, state.ProductLogsQuota)
	m.cfg = cfg
	m.onRCUpdate(quotaConfig(`"env:prod"`), noopApply)

	hostname, tags := m.enforcer.HostTags()
	assert.Equal(t, "my-host", hostname)
	assert.Equal(t, []string{"env:prod", "extra:x", "team:env-var"}, tags, "DD_TAGS overrides tags")
}

func TestQuotaHostTagsRefreshedBeforeRCUpdate(t *testing.T) {
	m := newTestQuotaManager(t, state.ProductLogsQuota)
	m.cfg.(model.Config).SetInTest("tags", []string{"team:a"})
	m.onRCUpdate(quotaConfig(`"team:a"`), noopApply)
	assert.True(t, m.enforcer.Check(newLocalMsg()))

	m.cfg.(model.Config).SetInTest("tags", []string{"team:b"})
	m.onRCUpdate(quotaConfig(`"team:b"`), noopApply)
	assert.True(t, m.enforcer.Check(newLocalMsg()), "new quotas use the tags sampled with them")
}

func TestQuotaHostTagsPolled(t *testing.T) {
	m := newTestQuotaManager(t, state.ProductLogsQuota)
	mock := m.clock.(*clock.Mock)
	cfg := m.cfg.(model.Config)
	m.onRCUpdate(quotaConfig(`"team:a"`), noopApply)
	require.False(t, m.enforcer.Check(newLocalMsg()))

	cfg.SetInTest("tags", []string{"team:a"})
	advanceUntil(t, mock, func() bool { return m.enforcer.Check(newLocalMsg()) })

	cfg.SetInTest("tags", []string{})
	advanceUntil(t, mock, func() bool { return !m.enforcer.Check(newLocalMsg()) })

	m.onRCUpdate(map[string]state.RawConfig{}, noopApply)
	assert.Eventually(t, func() bool {
		mock.Add(hostTagsPollEvery)
		m.mu.Lock()
		defer m.mu.Unlock()
		return !m.polling
	}, 5*time.Second, 10*time.Millisecond, "polling stops once no quota is active")
}

func TestQuotaConcurrentPollAndRCUpdate(t *testing.T) {
	m := newTestQuotaManager(t, state.ProductLogsQuota)
	mock := m.clock.(*clock.Mock)
	cfg := m.cfg.(model.Config)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 200 {
			mock.Add(hostTagsPollEvery)
		}
	}()
	for i := range 100 {
		cfg.SetInTest("tags", []string{fmt.Sprintf("team:%d", i)})
		m.onRCUpdate(quotaConfig(`"team:x"`), noopApply)
		_, tags := m.enforcer.HostTags()
		require.Equal(t, []string{fmt.Sprintf("team:%d", i)}, tags)
	}
	wg.Wait()
}

func TestQuotaStatusShowsHostTags(t *testing.T) {
	originalProvider := logsProvider
	logsProvider = func(bool) logsStatus.Status { return logsStatus.Status{IsRunning: true} }
	t.Cleanup(func() {
		logsProvider = originalProvider
		quota.Default().Update(map[string][]byte{})
		quota.Default().SetHostTags(nil, "")
	})
	quota.Default().SetHostTags([]string{"env:prod", "team:payments"}, "web-1")
	for _, c := range quotaConfig(`"env:prod","service:checkout"`) {
		quota.Default().Update(map[string][]byte{"q": c.Config})
	}

	var b bytes.Buffer
	require.NoError(t, StatusProvider{}.Text(false, &b))
	assert.Contains(t, b.String(), "Host tags for logs from web-1: env:prod,team:payments")
	assert.Contains(t, b.String(), "Tags: env:prod,service:checkout\n        From host tags: env:prod\n")
	t.Log(b.String())
}

func newLocalMsg() *message.Message {
	return message.NewMessageWithSource([]byte("x"), "", sources.NewLogSource("", &config.LogsConfig{}), 0)
}

func advanceUntil(t *testing.T, mock *clock.Mock, cond func() bool) {
	t.Helper()
	assert.Eventually(t, func() bool {
		mock.Add(hostTagsPollEvery)
		return cond()
	}, 5*time.Second, 10*time.Millisecond)
}
