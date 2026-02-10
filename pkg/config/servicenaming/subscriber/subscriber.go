// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

// Package subscriber provides a workloadmeta subscriber that evaluates CEL-based
// service naming rules against container metadata.
package subscriber

import (
	"context"
	"sync"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/servicenaming"
	"github.com/DataDog/datadog-agent/pkg/config/servicenaming/engine"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	subscriberName = "servicenaming-subscriber"
)

// Subscriber listens to workloadmeta container events and applies CEL-based
// service naming rules, storing results back into workloadmeta.
type Subscriber struct {
	wmeta  workloadmeta.Component
	ch     chan workloadmeta.EventBundle
	engine *engine.Engine

	// Protects serviceNameCache.
	mu sync.RWMutex

	// Last computed service name per container, used to avoid re-pushing identical results.
	serviceNameCache map[string]string
}

// NewSubscriber creates a new servicenaming subscriber.
// Returns nil if the feature is disabled or no rules are configured.
func NewSubscriber(cfg pkgconfigmodel.Reader, wmeta workloadmeta.Component) (*Subscriber, error) {
	sdConfig, err := servicenaming.LoadFromAgentConfig(cfg)
	if err != nil {
		return nil, err
	}

	// If config is inactive, do nothing
	if !sdConfig.IsActive() {
		log.Debug("CEL service naming is disabled or has no rules configured")
		return nil, nil
	}

	eng, err := sdConfig.CompileEngine()
	if err != nil {
		return nil, err
	}

	log.Infof("CEL service naming enabled with %d rules", len(sdConfig.ServiceDefinitions))

	sub := &Subscriber{
		wmeta:            wmeta,
		engine:           eng,
		serviceNameCache: make(map[string]string),
	}

	return sub, nil
}

// Start processes workloadmeta container events (call as goroutine: go sub.Start(ctx)).
func (s *Subscriber) Start(ctx context.Context) {
	if s.ch == nil {
		// Subscribe to all container events (Set and Unset) from all sources.
		// Workloadmeta already filters duplicate events using reflect.DeepEqual,
		// so we only receive events when container metadata actually changed.
		filter := workloadmeta.NewFilterBuilder().
			SetSource(workloadmeta.SourceAll).
			AddKind(workloadmeta.KindContainer).
			Build()

		s.ch = s.wmeta.Subscribe(subscriberName, workloadmeta.NormalPriority, filter)
		log.Debug("servicenaming subscriber subscribed to workloadmeta container events")
	}

	log.Debug("servicenaming subscriber event loop started")

	for {
		select {
		case <-ctx.Done():
			s.wmeta.Unsubscribe(s.ch)
			log.Debug("servicenaming subscriber stopped")
			return

		case bundle, ok := <-s.ch:
			if !ok {
				// Channel closed
				return
			}
			s.handleEvents(ctx, bundle.Events)
			bundle.Acknowledge()
		}
	}
}

// handleEvents processes container Set/Unset events.
func (s *Subscriber) handleEvents(ctx context.Context, events []workloadmeta.Event) {
	for _, event := range events {
		container, ok := event.Entity.(*workloadmeta.Container)
		if !ok {
			continue
		}

		switch event.Type {
		case workloadmeta.EventTypeSet:
			s.processContainer(ctx, container)

		case workloadmeta.EventTypeUnset:
			log.Debugf("CEL service naming: container %s being deleted, cleaning up service discovery", container.ID)
			s.clearServiceDiscovery(container)
			s.removeFromCache(container.ID)
		}
	}
}

func (s *Subscriber) removeFromCache(containerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.serviceNameCache, containerID)
}

// processContainer evaluates CEL rules and updates workloadmeta with the result.
// Note: workloadmeta already filters duplicate events using reflect.DeepEqual,
// so this function only receives events when container metadata actually changed.
func (s *Subscriber) processContainer(_ context.Context, container *workloadmeta.Container) {
	input := buildCELInput(container)
	engineInput := servicenaming.ToEngineInput(input)

	result := s.engine.Evaluate(engineInput)

	if result == nil {
		log.Debugf("CEL service naming: no rule matched for container %s (name=%s, image=%s, labels=%d)",
			container.ID, container.Name, container.Image.ShortName, len(container.Labels))

		if container.CELServiceDiscovery != nil {
			log.Debugf("CEL service naming: clearing stale service discovery for container %s", container.ID)
			s.clearServiceDiscovery(container)
			s.removeFromCache(container.ID)
		}
		return
	}

	// Check if we already computed this exact result to avoid redundant pushes
	s.mu.RLock()
	cachedServiceName, exists := s.serviceNameCache[container.ID]
	s.mu.RUnlock()

	if exists && cachedServiceName == result.ServiceName {
		// We already pushed this service name and the container still matches
		log.Tracef("CEL service naming: container %s already has service name %q, skipping push", container.ID, result.ServiceName)
		return
	}

	log.Debugf("CEL service naming: container %s matched rule %q, service name: %s",
		container.ID, result.MatchedRule, result.ServiceName)

	// Update cache before push to prevent race condition
	s.mu.Lock()
	s.serviceNameCache[container.ID] = result.ServiceName
	s.mu.Unlock()

	err := s.wmeta.Push(workloadmeta.SourceServiceDiscovery, workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.Container{
			EntityID: container.EntityID,
			CELServiceDiscovery: &workloadmeta.CELServiceDiscovery{
				ServiceName: result.ServiceName,
				MatchedRule: result.MatchedRule,
			},
		},
	})
	if err != nil {
		log.Warnf("Failed to push CEL service discovery result for container %s: %v", container.ID, err)
		s.removeFromCache(container.ID)
	}
}

func (s *Subscriber) clearServiceDiscovery(container *workloadmeta.Container) {
	log.Debugf("CEL service naming: clearing service discovery for container %s", container.ID)

	err := s.wmeta.Push(workloadmeta.SourceServiceDiscovery, workloadmeta.Event{
		Type: workloadmeta.EventTypeUnset,
		Entity: &workloadmeta.Container{
			EntityID: container.EntityID,
		},
	})
	if err != nil {
		log.Warnf("Failed to clear CEL service discovery for container %s: %v", container.ID, err)
	}
}

// buildCELInput converts workloadmeta.Container to servicenaming.CELInput.
func buildCELInput(container *workloadmeta.Container) servicenaming.CELInput {
	ports := make([]servicenaming.PortCEL, len(container.Ports))
	for i, p := range container.Ports {
		ports[i] = servicenaming.PortCEL{
			Name:     p.Name,
			Port:     p.Port,
			Protocol: p.Protocol,
		}
	}

	labels := container.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	envs := container.EnvVars
	if envs == nil {
		envs = map[string]string{}
	}

	return servicenaming.CELInput{
		Container: &servicenaming.ContainerCEL{
			ID:   container.ID,
			Name: container.Name,
			Image: servicenaming.ImageCEL{
				Name:      container.Image.Name,
				ShortName: container.Image.ShortName,
				Tag:       container.Image.Tag,
				Registry:  container.Image.Registry,
			},
			Labels: labels,
			Envs:   envs,
			Ports:  ports,
		},
	}
}
