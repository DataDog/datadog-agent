// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"context"
	"os"
	"path/filepath"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/quota"
	hosttags "github.com/DataDog/datadog-agent/comp/metadata/host/impl/hosttags"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const (
	quotaEnabledKey   = "logs_config.quota_enforcement.enabled"
	quotaMockDirKey   = "logs_config.quota_enforcement.mock_rc_dir"
	quotaSourceRC     = "rc"
	quotaSourceMock   = "mock"
	quotaMockInterval = 2 * time.Second
	hostTagsRefresh   = 10 * time.Minute
)

type quotaManager struct {
	log      log.Component
	config   model.Reader
	enforcer *quota.Enforcer
	cancel   context.CancelFunc
	done     chan struct{}
}

func newQuotaManager(logger log.Component, cfg model.Reader) *quotaManager {
	return &quotaManager{log: logger, config: cfg, enforcer: quota.Default()}
}

func (m *quotaManager) rcListener() rctypes.ListenerProvider {
	return rctypes.ListenerProvider{
		ListenerProvider: rctypes.RCListener{state.ProductLogsQuota: m.onRCUpdate},
	}
}

func (m *quotaManager) onRCUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	configs := make(map[string][]byte, len(updates))
	for path, u := range updates {
		configs[path] = u.Config
	}
	for path, err := range m.enforcer.Update(quotaSourceRC, configs) {
		if err != nil {
			m.log.Warnf("Rejected LOGS_QUOTA config %s: %v", path, err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}

func (m *quotaManager) start(context.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.done = make(chan struct{})
	go m.run(ctx, m.config.GetString(quotaMockDirKey))
	return nil
}

func (m *quotaManager) stop(context.Context) error {
	if m.cancel != nil {
		m.cancel()
		<-m.done
	}
	return nil
}

func (m *quotaManager) run(ctx context.Context, mockDir string) {
	defer close(m.done)

	m.refreshHostTags(ctx, false)
	hostTicker := time.NewTicker(hostTagsRefresh)
	defer hostTicker.Stop()

	var mockTick <-chan time.Time
	var last map[string][]byte
	if mockDir != "" {
		m.log.Infof("Logs quota enforcement: reading mock LOGS_QUOTA configs from %s", mockDir)
		t := time.NewTicker(quotaMockInterval)
		defer t.Stop()
		mockTick = t.C
		last = m.pollMockDir(mockDir, nil)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-hostTicker.C:
			m.refreshHostTags(ctx, true)
		case <-mockTick:
			last = m.pollMockDir(mockDir, last)
		}
	}
}

func (m *quotaManager) refreshHostTags(ctx context.Context, cached bool) {
	tags := hosttags.Get(ctx, cached, m.config).System
	m.enforcer.SetHostTagsProvider(func() []string { return tags })
}

// pollMockDir feeds each *.json file in dir to the enforcer as a LOGS_QUOTA config.
func (m *quotaManager) pollMockDir(dir string, last map[string][]byte) map[string][]byte {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		m.log.Warnf("Logs quota mock: %v", err)
		return last
	}
	configs := make(map[string][]byte, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			m.log.Warnf("Logs quota mock: %v", err)
			continue
		}
		configs[filepath.Base(f)] = b
	}
	if last != nil && sameConfigs(last, configs) {
		return last
	}

	for path, err := range m.enforcer.Update(quotaSourceMock, configs) {
		if err != nil {
			m.log.Warnf("Logs quota mock: rejected %s: %v", path, err)
		} else {
			m.log.Infof("Logs quota mock: acknowledged %s", path)
		}
	}
	return configs
}

func sameConfigs(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || string(v) != string(w) {
			return false
		}
	}
	return true
}
