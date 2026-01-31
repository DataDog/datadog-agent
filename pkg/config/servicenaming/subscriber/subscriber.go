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
	"hash/fnv"
	"sort"
	"strconv"

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
	ctx    context.Context // Stored context for evaluation cancellation

	// lastComputed tracks the last service name we computed for each container.
	// Used with lastInputHash to skip re-evaluation when we receive our own events.
	lastComputed map[string]string

	// lastInputHash tracks a hash of the container metadata (labels, envs, image)
	// that was used to compute the service name. Combined with lastComputed, this
	// allows us to detect when container metadata has changed vs when we're just
	// receiving our own SourceServiceDiscovery event back.
	lastInputHash map[string]uint64
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

	log.Infof("CEL service naming enabled with %d rules", len(sdConfig.ServiceDefinitions))

	return &Subscriber{
		engine:        eng,
		wmeta:         wmeta,
		lastComputed:  make(map[string]string),
		lastInputHash: make(map[string]uint64),
	}, nil
}

// Start begins processing workloadmeta container events.
// This method handles subscription internally and should be called as a goroutine: go subscriber.Start(ctx)
// The context is used for both shutdown signaling and CEL evaluation cancellation.
func (s *Subscriber) Start(ctx context.Context) {
	// Store context for evaluation cancellation
	s.ctx = ctx

	// Subscribe to workloadmeta events if not already subscribed
	if s.ch == nil {
		// Subscribe to SourceAll to receive updates from all sources (runtime, orchestrators, etc.).
		// We filter out our own SourceServiceDiscovery events in handleEvents to avoid self-triggering.
		// This ensures we react to label/env changes from SourceNodeOrchestrator (kubelet, ECS).
		filter := workloadmeta.NewFilterBuilder().
			SetSource(workloadmeta.SourceAll).
			AddKind(workloadmeta.KindContainer).
			Build() // No event type filter - handle both Set and Unset

		s.ch = s.wmeta.Subscribe(subscriberName, workloadmeta.NormalPriority, filter)
		log.Debug("servicenaming subscriber subscribed to workloadmeta events (all sources)")
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
			s.handleEvents(bundle.Events)
			bundle.Acknowledge()
		}
	}
}

// handleEvents processes a batch of container events.
// Handles both Set (create/update) and Unset (delete) events.
// Since we subscribe to SourceAll, events contain the merged entity from all sources,
// including our CELServiceDiscovery field if it exists. This allows proper idempotency checks.
func (s *Subscriber) handleEvents(events []workloadmeta.Event) {
	for _, event := range events {
		container, ok := event.Entity.(*workloadmeta.Container)
		if !ok {
			continue
		}

		switch event.Type {
		case workloadmeta.EventTypeSet:
			// Container created or updated from any source - evaluate rules.
			// The merged entity includes CELServiceDiscovery from our previous evaluation.
			s.processContainer(container)

		case workloadmeta.EventTypeUnset:
			// Container being deleted - clean up our service discovery contribution.
			// EventTypeUnset is only sent when the last source removes the entity.
			// If only our SourceServiceDiscovery remains, we'll receive EventTypeUnset and must clean up
			// to prevent stale data from keeping the entity alive.
			log.Debugf("CEL service naming: container %s being deleted, cleaning up service discovery", container.ID)
			s.clearServiceDiscovery(container)
			delete(s.lastComputed, container.ID)  // Clean up tracking
			delete(s.lastInputHash, container.ID) // Clean up hash tracking
		}
	}
}

// processContainer evaluates CEL rules for a single container and stores the result.
// If a rule matches, the computed service name is stored back into workloadmeta.
// If no rule matches, any existing CELServiceDiscovery is cleared to prevent stale data.
// Pushes are skipped if the computed value matches what's already in workloadmeta.
//
// Note: Because we subscribe to SourceAll, the container parameter contains the merged
// entity from all sources, including our CELServiceDiscovery field if previously set.
// This enables proper idempotency checks and cleanup detection.
func (s *Subscriber) processContainer(container *workloadmeta.Container) {
	// Compute hash of container input to detect changes
	currentInputHash := hashContainerInput(container)

	// Fast path: Skip expensive CEL re-evaluation if both the input (container metadata)
	// and output (service name) are unchanged. This filters out our own SourceServiceDiscovery
	// events coming back without skipping legitimate updates from other sources.
	if container.CELServiceDiscovery != nil {
		if lastService, exists := s.lastComputed[container.ID]; exists {
			if lastHash, hashExists := s.lastInputHash[container.ID]; hashExists {
				if container.CELServiceDiscovery.ServiceName == lastService && currentInputHash == lastHash {
					// Input unchanged + output unchanged = our own event coming back
					log.Tracef("CEL service naming: skipping re-evaluation for container %s (input and output unchanged)", container.ID)
					return
				}
			}
		}
	}

	// Build CELInput from container metadata
	input := buildCELInput(container)

	// Convert to engine input format
	engineInput := servicenaming.ToEngineInput(input)

	// Evaluate rules with subscriber's context (respects shutdown cancellation)
	result := s.engine.Evaluate(s.ctx, engineInput)

	if result == nil {
		// No rule matched - need to clear any stale CELServiceDiscovery data.
		// The merged entity includes CELServiceDiscovery from our previous evaluation,
		// so we can detect if there's stale data to clear.
		log.Debugf("CEL service naming: no rule matched for container %s (name=%s, image=%s, labels=%d)",
			container.ID, container.Name, container.Image.ShortName, len(container.Labels))

		// Only clear if there's actually stale data to remove
		if container.CELServiceDiscovery != nil {
			log.Debugf("CEL service naming: clearing stale service discovery for container %s", container.ID)
			s.clearServiceDiscovery(container)
			delete(s.lastComputed, container.ID)  // Remove from tracking
			delete(s.lastInputHash, container.ID) // Remove hash tracking
		}
		return
	}

	// Check if we need to update (avoid redundant pushes)
	if container.CELServiceDiscovery != nil {
		existing := container.CELServiceDiscovery
		if existing.ServiceName == result.ServiceName && existing.MatchedRule == result.MatchedRule {
			// No change - skip push, but update tracking since we just evaluated
			log.Tracef("CEL service naming: container %s already has correct service name %q", container.ID, result.ServiceName)
			s.lastComputed[container.ID] = result.ServiceName
			s.lastInputHash[container.ID] = currentInputHash
			return
		}
	}

	log.Debugf("CEL service naming: container %s matched rule %q, service name: %s",
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
	} else {
		// Track this computation and input hash to skip re-evaluation when we receive our own event
		s.lastComputed[container.ID] = result.ServiceName
		s.lastInputHash[container.ID] = currentInputHash
	}
}

// clearServiceDiscovery removes CEL service discovery data for a container.
// This is called when no rule matches anymore and we need to remove our stale contribution.
// Uses EventTypeUnset to completely remove the SourceServiceDiscovery contribution from workloadmeta.
func (s *Subscriber) clearServiceDiscovery(container *workloadmeta.Container) {
	log.Debugf("CEL service naming: clearing service discovery for container %s", container.ID)

	// Push EventTypeUnset to remove our SourceServiceDiscovery contribution entirely.
	// This ensures stale CELServiceDiscovery data doesn't persist.
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

// hashContainerInput computes a hash of the container metadata fields that affect
// CEL rule evaluation. This is used to detect when container metadata has actually
// changed vs when we're just receiving our own SourceServiceDiscovery event back.
func hashContainerInput(container *workloadmeta.Container) uint64 {
	h := fnv.New64a()

	// Hash image fields
	h.Write([]byte(container.Image.Name))
	h.Write([]byte(container.Image.ShortName))
	h.Write([]byte(container.Image.Tag))
	h.Write([]byte(container.Image.Registry))

	// Hash labels (sorted keys for consistency)
	if container.Labels != nil {
		keys := make([]string, 0, len(container.Labels))
		for k := range container.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := container.Labels[k]
			h.Write([]byte(k))
			h.Write([]byte(v))
		}
	}

	// Hash envs (sorted keys for consistency)
	if container.EnvVars != nil {
		keys := make([]string, 0, len(container.EnvVars))
		for k := range container.EnvVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := container.EnvVars[k]
			h.Write([]byte(k))
			h.Write([]byte(v))
		}
	}

	// Hash ports (sorted for consistency)
	if len(container.Ports) > 0 {
		ports := make([]workloadmeta.ContainerPort, len(container.Ports))
		copy(ports, container.Ports)
		sort.Slice(ports, func(i, j int) bool {
			if ports[i].Port != ports[j].Port {
				return ports[i].Port < ports[j].Port
			}
			if ports[i].Protocol != ports[j].Protocol {
				return ports[i].Protocol < ports[j].Protocol
			}
			return ports[i].Name < ports[j].Name
		})
		for _, p := range ports {
			h.Write([]byte(p.Name))
			h.Write([]byte(p.Protocol))
			h.Write([]byte(strconv.Itoa(p.Port)))
		}
	}

	// Hash container name
	h.Write([]byte(container.Name))

	return h.Sum64()
}

// buildCELInput creates a servicenaming.CELInput from a workloadmeta.Container.
// This function maps workloadmeta container metadata to the CEL input structure,
// including image info, labels, environment variables, and ports.
// Normalizes nil maps to empty maps to ensure consistent CEL behavior.
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

	// Normalize nil maps to empty maps for consistent CEL behavior
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
