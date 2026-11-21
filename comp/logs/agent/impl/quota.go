// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"encoding/json"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/quota"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const (
	quotaEnabledKey   = "logs_config.quota_enforcement.enabled"
	quotaRCProductKey = "logs_config.quota_enforcement.rc_product"
)

type quotaManager struct {
	log      log.Component
	enforcer *quota.Enforcer
	product  string
}

func newQuotaManager(logger log.Component, cfg model.Reader) *quotaManager {
	return &quotaManager{log: logger, enforcer: quota.Default(), product: cfg.GetString(quotaRCProductKey)}
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
	for path, err := range m.enforcer.Update(configs) {
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
