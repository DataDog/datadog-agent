package haagentimpl

import (
	"context"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/atomic"
)

type haAgentImpl struct {
	log            log.Component
	haAgentConfigs *haAgentConfigs
	isLeader       *atomic.Bool
}

func newHaAgentImpl(log log.Component, haAgentConfigs *haAgentConfigs) *haAgentImpl {
	return &haAgentImpl{
		log:            log,
		haAgentConfigs: haAgentConfigs,
		isLeader:       atomic.NewBool(false),
	}
}

func (h *haAgentImpl) Enabled() bool {
	return h.haAgentConfigs.enabled
}

func (h *haAgentImpl) GetGroup() string {
	return h.haAgentConfigs.group
}

func (h *haAgentImpl) IsLeader() bool {
	return h.isLeader.Load()
}

func (h *haAgentImpl) SetLeader(leaderAgentHostname string) {
	agentHostname, err := hostname.Get(context.TODO())
	if err != nil {
		h.log.Warnf("Error getting the hostname: %v", err)
		return
	}
	h.isLeader.Store(agentHostname == leaderAgentHostname)
}
