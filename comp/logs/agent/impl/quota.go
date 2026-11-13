// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/quota"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const (
	quotaEnabledKey   = "logs_config.quota_enforcement.enabled"
	quotaRCProductKey = "logs_config.quota_enforcement.rc_product"
)

const hostTagsPollEvery = time.Minute

type quotaManager struct {
	log      log.Component
	cfg      model.Reader
	hostname hostname.Component
	enforcer *quota.Enforcer
	product  string
	clock    clock.Clock

	// mu serializes host tag refreshes with quota updates so an older sample never lands last.
	mu      sync.Mutex
	polling bool
}

func newQuotaManager(logger log.Component, cfg model.Reader, hostnameComp hostname.Component) *quotaManager {
	m := &quotaManager{
		log:      logger,
		cfg:      cfg,
		hostname: hostnameComp,
		enforcer: quota.Default(),
		product:  cfg.GetString(quotaRCProductKey),
		clock:    clock.New(),
	}
	m.mu.Lock()
	m.refreshHostTags()
	m.mu.Unlock()
	return m
}

// refreshHostTags passes the locally configured host tags to the enforcer. m.mu must be held.
func (m *quotaManager) refreshHostTags() {
	tags := configUtils.GetConfiguredTags(m.cfg, false)
	if env := m.cfg.GetString("env"); env != "" {
		tags = append(tags, "env:"+env)
	}
	name, err := m.hostname.Get(context.Background())
	if err != nil {
		m.log.Debugf("Logs quota enforcement: no hostname, host tags apply only to logs without one: %v", err)
	}
	m.enforcer.SetHostTags(tags, name)
}

func (m *quotaManager) startPolling() {
	if m.polling || !m.enforcer.Active() {
		return
	}
	m.polling = true
	go m.poll()
}

func (m *quotaManager) poll() {
	t := m.clock.Ticker(hostTagsPollEvery)
	defer t.Stop()
	for range t.C {
		if !m.pollOnce() {
			return
		}
	}
}

// pollOnce refreshes the host tags and reports whether quotas are still active.
func (m *quotaManager) pollOnce() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.enforcer.Active() {
		m.polling = false
		return false
	}
	m.refreshHostTags()
	return true
}

func (m *quotaManager) rcListener() rctypes.ListenerProvider {
	m.log.Infof("Logs quota enforcement: subscribing to Remote Config product %s", m.product)
	return rctypes.ListenerProvider{
		ListenerProvider: rctypes.RCListener{data.Product(m.product): m.onRCUpdate},
	}
}

func (m *quotaManager) onRCUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	configs := make(map[string][]byte, len(updates))
	for path, u := range updates {
		// DEBUG is shared by every staging PoC; only take configs shaped like ours.
		if m.product == state.ProductDebug && !hasQuotasKey(u.Config) {
			m.log.Debugf("Ignoring DEBUG config %s: no quotas key", path)
			continue
		}
		configs[path] = u.Config
	}
	m.mu.Lock()
	m.refreshHostTags()
	results := m.enforcer.Update(configs)
	m.startPolling()
	m.mu.Unlock()

	for path, err := range results {
		if err != nil {
			m.log.Warnf("Rejected log quota config %s: %v", path, err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}

func hasQuotasKey(raw []byte) bool {
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return false
	}
	_, ok := top["quotas"]
	return ok
}
