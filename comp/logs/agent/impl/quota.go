// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"context"
	"encoding/json"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/quota"
	hosttags "github.com/DataDog/datadog-agent/comp/metadata/host/impl/hosttags"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const (
	quotaEnabledKey   = "logs_config.quota_enforcement.enabled"
	quotaRCProductKey = "logs_config.quota_enforcement.rc_product"
	quotaSourceRC     = "rc"
	hostTagsRefresh   = 10 * time.Minute
)

type quotaManager struct {
	log      log.Component
	config   model.Reader
	enforcer *quota.Enforcer
	product  string
	cancel   context.CancelFunc
	done     chan struct{}
}

func newQuotaManager(logger log.Component, cfg model.Reader) *quotaManager {
	return &quotaManager{log: logger, config: cfg, enforcer: quota.Default(), product: cfg.GetString(quotaRCProductKey)}
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
		if m.product != state.ProductDebug {
			configs[path] = u.Config
			continue
		}
		// DEBUG is shared by every staging PoC; only take configs wrapped for us.
		var wrapper struct {
			LogsQuota json.RawMessage `json:"logs_quota"`
		}
		if json.Unmarshal(u.Config, &wrapper) != nil || len(wrapper.LogsQuota) == 0 {
			m.log.Debugf("Ignoring DEBUG config %s: no logs_quota key", path)
			continue
		}
		configs[path] = wrapper.LogsQuota
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
	go m.run(ctx)
	return nil
}

func (m *quotaManager) stop(context.Context) error {
	if m.cancel != nil {
		m.cancel()
		<-m.done
	}
	return nil
}

func (m *quotaManager) run(ctx context.Context) {
	defer close(m.done)

	m.refreshHostTags(ctx, false)
	hostTicker := time.NewTicker(hostTagsRefresh)
	defer hostTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hostTicker.C:
			m.refreshHostTags(ctx, true)
		}
	}
}

func (m *quotaManager) refreshHostTags(ctx context.Context, cached bool) {
	tags := hosttags.Get(ctx, cached, m.config).System
	m.enforcer.SetHostTagsProvider(func() []string { return tags })
}
