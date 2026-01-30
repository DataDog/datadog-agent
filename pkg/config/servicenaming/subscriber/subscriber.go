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

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/servicenaming"
	"github.com/DataDog/datadog-agent/pkg/config/servicenaming/engine"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const subscriberName = "servicenaming-subscriber"

// Subscriber listens to workloadmeta container events and applies CEL-based
// service naming rules, storing computed service names back into workloadmeta.
type Subscriber struct {
	engine *engine.Engine
	wmeta  workloadmeta.Component
	ch     chan workloadmeta.EventBundle
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

	// Compile the engine
	eng, err := sdConfig.CompileEngine()
	if err != nil {
		return nil, err
	}

	if eng == nil {
		log.Debug("CEL service naming: engine is nil (no active rules)")
		return nil, nil
	}

	log.Info("CEL service naming enabled with ", len(sdConfig.ServiceDefinitions), " rules")

	return &Subscriber{
		engine: eng,
		wmeta:  wmeta,
	}, nil
}

// Start begins subscribing to workloadmeta container events.
// This should be called as a goroutine: go subscriber.Start(ctx)
func (s *Subscriber) Start(ctx context.Context) {
	// Subscribe to container set events only
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceAll).
		AddKind(workloadmeta.KindContainer).
		SetEventType(workloadmeta.EventTypeSet).
		Build()

	s.ch = s.wmeta.Subscribe(subscriberName, workloadmeta.NormalPriority, filter)

	log.Debug("servicenaming subscriber started")

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
			s.handleEvents(bundle.Events)
			bundle.Acknowledge()
		}
	}
}

// handleEvents processes a batch of container events.
// Only EventTypeSet events for containers are processed; others are skipped.
func (s *Subscriber) handleEvents(events []workloadmeta.Event) {
	for _, event := range events {
		if event.Type != workloadmeta.EventTypeSet {
			continue
		}

		container, ok := event.Entity.(*workloadmeta.Container)
		if !ok {
			continue
		}

		s.processContainer(container)
	}
}

// processContainer evaluates CEL rules for a single container and stores the result.
// If a rule matches, the computed service name is stored back into workloadmeta
// as CELServiceDiscovery metadata. If no rule matches, no metadata is written.
func (s *Subscriber) processContainer(container *workloadmeta.Container) {
	// Build CELInput from container metadata
	input := buildCELInput(container)

	// Convert to engine input format
	engineInput := servicenaming.ToEngineInput(input)

	// Evaluate rules
	result := s.engine.Evaluate(engineInput)

	if result == nil {
		// No rule matched, nothing to store
		return
	}

	log.Debugf("CEL service naming: container %s matched rule '%s', service name: %s",
		container.ID, result.MatchedRule, result.ServiceName)

	// Store the computed service name back into workloadmeta as derived metadata
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
	}
}

// buildCELInput creates a servicenaming.CELInput from a workloadmeta.Container.
// This function maps workloadmeta container metadata to the CEL input structure,
// including image info, labels, environment variables, and ports.
func buildCELInput(container *workloadmeta.Container) servicenaming.CELInput {
	// Map ports from workloadmeta format to CEL format
	ports := make([]servicenaming.PortCEL, len(container.Ports))
	for i, p := range container.Ports {
		ports[i] = servicenaming.PortCEL{
			Name:     p.Name,
			Port:     p.Port,
			Protocol: p.Protocol,
		}
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
			Labels: container.Labels,
			Envs:   container.EnvVars,
			Ports:  ports,
		},
	}
}
