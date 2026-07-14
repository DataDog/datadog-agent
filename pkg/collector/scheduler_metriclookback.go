// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python || test

package collector

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	collectoraggregator "github.com/DataDog/datadog-agent/pkg/collector/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/lookbacksender"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type shadowCandidate = metriclookback.ShadowCandidate

func shadowCandidatesByInstance(config integration.Config) map[int]shadowCandidate {
	candidates := metriclookback.SelectShadowCandidates([]integration.Config{config}, metriclookback.ShadowPolicyOptionsFromConfig(setup.Datadog()))
	if len(candidates) == 0 {
		return nil
	}
	byInstance := make(map[int]shadowCandidate, len(candidates))
	for _, candidate := range candidates {
		byInstance[candidate.InstanceIndex] = candidate
	}
	return byInstance
}

func (s *CheckScheduler) ensureShadowSenderContext() context.Context {
	if s.shadowSenderContext == nil {
		s.shadowSenderContext, s.shadowSenderCancel = context.WithCancel(context.Background())
	}
	return s.shadowSenderContext
}

func (s *CheckScheduler) loadShadowCheck(candidate shadowCandidate, loader check.Loader, sourceCheckID checkid.ID) (check.Check, error) {
	shadowSenderManager := s.shadowSenderManager
	if shadowSenderManager == nil {
		shadowSenderManager = lookbacksender.NewSenderManager(s.ensureShadowSenderContext(), "", nil, nil)
		s.shadowSenderManager = shadowSenderManager
	}
	shadowCheckID := check.ShadowID(sourceCheckID)
	checkSenderManager := &shadowCheckSenderManager{
		SenderManager: shadowSenderManager,
		shadowCheckID: shadowCheckID,
	}
	loadedCheck, err := loader.Load(checkSenderManager, candidate.SourceConfig, candidate.Instance, candidate.InstanceIndex)
	if err != nil {
		checkSenderManager.DestroySender(shadowCheckID)
		return nil, err
	}
	if !checkSenderManager.RegisterCallbackID(loadedCheck.ID()) {
		log.Warnf("Unable to register metric lookback rtloader callback route for shadow check %s loaded as %s", shadowCheckID, loadedCheck.ID())
	}
	s.applyInfraTagger(checkSenderManager, candidate.SourceConfig.Name, shadowCheckID)
	return check.NewShadowCheckForSource(loadedCheck, sourceCheckID, candidate.ShadowInterval, checkSenderManager), nil
}

type shadowCheckSenderManager struct {
	sender.SenderManager
	shadowCheckID       checkid.ID
	unregisterCallbacks []func()
}

func (m shadowCheckSenderManager) GetSender(checkid.ID) (sender.Sender, error) {
	return m.SenderManager.GetSender(m.shadowCheckID)
}

func (m shadowCheckSenderManager) SetSender(s sender.Sender, _ checkid.ID) error {
	return m.SenderManager.SetSender(s, m.shadowCheckID)
}

func (m *shadowCheckSenderManager) DestroySender(checkid.ID) {
	for _, unregister := range m.unregisterCallbacks {
		unregister()
	}
	m.unregisterCallbacks = nil
	m.SenderManager.DestroySender(m.shadowCheckID)
}

func (m *shadowCheckSenderManager) RegisterCallbackID(id checkid.ID) bool {
	unregister, ok := collectoraggregator.RegisterCheckSenderManager(id, m)
	if !ok {
		return false
	}
	m.unregisterCallbacks = append(m.unregisterCallbacks, unregister)
	return true
}
