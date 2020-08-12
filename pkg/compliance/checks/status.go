// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// status maintains status updates for all configured checks
type status struct {
	ruleIDs []string
	checks  map[string]*compliance.CheckStatus
	mu      sync.RWMutex
}

func newStatus() *status {
	return &status{
		checks: make(map[string]*compliance.CheckStatus),
	}
}

func (s *status) addCheck(checkStatus *compliance.CheckStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ruleIDs = append(s.ruleIDs, checkStatus.RuleID)
	s.checks[checkStatus.RuleID] = checkStatus
}

func (s *status) updateCheck(ruleID string, event *event.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats, ok := s.checks[ruleID]
	if !ok {
		log.Errorf("Check with ruleID=%s is not registered in check state", ruleID)
		return
	}
	stats.LastEvent = event
}

func (s *status) getChecksStatus() compliance.CheckStatusList {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var checks []*compliance.CheckStatus
	for _, ruleID := range s.ruleIDs {
		if c, ok := s.checks[ruleID]; ok {
			checks = append(checks, c)
		}
	}
	return checks
}
