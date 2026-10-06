// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	metriclookbackdef "github.com/DataDog/datadog-agent/comp/metriclookback/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/checkcontext"
	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewShadowCheckFactory creates scheduler-local metric lookback support. A nil
// sender manager omits the capability, rather than installing a no-op factory.
func NewShadowCheckFactory(cfg model.Reader, manager sender.SenderManager) metriclookbackdef.ShadowCheckFactory {
	if manager == nil {
		return nil
	}
	return &shadowCheckFactory{config: cfg, shadowSenderManager: manager}
}

// The owning scheduler serializes Prepare and the returned loader calls.
type shadowCheckFactory struct {
	config              model.Reader
	shadowSenderManager sender.SenderManager
	shadowCoreLoader    check.Loader
}

func (f *shadowCheckFactory) Prepare(config integration.Config) map[int]metriclookbackdef.ShadowCheckLoader {
	candidates := SelectShadowCandidates([]integration.Config{config}, ShadowPolicyOptionsFromConfig(f.config))
	if len(candidates) == 0 {
		return nil
	}
	loaders := make(map[int]metriclookbackdef.ShadowCheckLoader, len(candidates))
	for _, candidate := range candidates {
		loaders[candidate.InstanceIndex] = func(loader check.Loader, sourceCheckID checkid.ID) (check.Check, error) {
			shadowLoader, ok := f.shadowLoaderFor(loader)
			if !ok {
				log.Debugf("Skipping metric lookback shadow check %s: loader %s does not support shadow execution", check.ShadowID(sourceCheckID), loader.Name())
				return nil, nil
			}
			return f.loadShadowCheck(candidate, shadowLoader, sourceCheckID)
		}
	}
	return loaders
}

func (f *shadowCheckFactory) loadShadowCheck(candidate ShadowCandidate, loader check.Loader, sourceCheckID checkid.ID) (check.Check, error) {
	shadowSenderManager := f.shadowSenderManager
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
	return check.NewShadowCheckForSource(loadedCheck, sourceCheckID, candidate.ShadowInterval, checkSenderManager), nil
}

func (f *shadowCheckFactory) shadowLoaderFor(loader check.Loader) (check.Loader, bool) {
	switch loader.Name() {
	case corecheckLoader.GoCheckLoaderName:
		if f.shadowCoreLoader != nil {
			return f.shadowCoreLoader, true
		}
		shadowLoader, err := corecheckLoader.NewGoCheckLoader(corecheckLoader.WithLoadMode(corecheckLoader.ShadowLoadMode))
		if err != nil {
			log.Debugf("Unable to create metric lookback shadow loader for %s: %v", loader.Name(), err)
			return nil, false
		}
		f.shadowCoreLoader = shadowLoader
		return shadowLoader, true
	case "python":
		return loader, true
	default:
		return nil, false
	}
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
	unregister, ok := checkcontext.RegisterCheckSenderManager(id, m)
	if !ok {
		return false
	}
	m.unregisterCallbacks = append(m.unregisterCallbacks, unregister)
	return true
}
