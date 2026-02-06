// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel && servicenaming

// Package subscriber provides a workloadmeta subscriber that evaluates CEL-based
// service naming rules against container metadata.
package subscriber

import (
	"context"
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/servicenaming"
	"github.com/DataDog/datadog-agent/pkg/config/servicenaming/engine"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	subscriberName = "servicenaming-subscriber"

	// cleanupInterval defines how often we check for stale container entries in our cache.
	// This prevents unbounded memory growth if containers are removed without proper Unset events.
	cleanupInterval = 10 * time.Minute

	// maxLabelsForHashing limits the number of labels processed during input hashing.
	// This prevents DoS attacks from containers with excessive labels.
	maxLabelsForHashing = 1000

	// maxEnvsForHashing limits the number of env vars processed during input hashing.
	maxEnvsForHashing = 1000

	// maxLabelValueLen limits the length of individual label/env values during hashing.
	// Values exceeding this are truncated to prevent memory issues.
	maxLabelValueLen = 10 * 1024 // 10KB
)

// Subscriber listens to workloadmeta container events and applies CEL-based
// service naming rules, storing computed service names back into workloadmeta.
type Subscriber struct {
	cfg   pkgconfigmodel.Reader
	wmeta workloadmeta.Component
	ch    chan workloadmeta.EventBundle

	// engineMu protects the engine field from concurrent access during hot-reload.
	engineMu sync.RWMutex
	engine   *engine.Engine

	// mu protects serviceNameCache and inputHashCache from concurrent access.
	mu sync.RWMutex

	// serviceNameCache tracks the last service name computed for each container.
	// Protected by mu.
	serviceNameCache map[string]string

	// inputHashCache tracks a hash of the container metadata (labels, envs, image)
	// used to compute the service name. This detects when container metadata changes.
	// Protected by mu.
	inputHashCache map[string]uint64

	// started tracks whether Start() has been called. This prevents config callbacks
	// from firing before the subscriber is fully initialized and ready to process events.
	started atomic.Bool
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
		cfg:              cfg,
		wmeta:            wmeta,
		engine:           eng,
		serviceNameCache: make(map[string]string),
		inputHashCache:   make(map[string]uint64),
	}

	// Register config change callback for hot-reload support
	// TODO: The config system (pkgconfigmodel.Reader.OnUpdate) doesn't provide a way to
	// unregister callbacks, which causes a memory leak if the subscriber is stopped and
	// recreated (e.g., in tests or if future agent features support dynamic component reloading).
	// For MVP, this is acceptable because:
	//   1. The subscriber lives as long as the agent process (typically weeks/months)
	//   2. Config reloads are infrequent (manual updates only)
	//   3. Memory leaked per callback registration is minimal (~hundreds of bytes)
	// For production hardening, consider:
	//   - Adding an unsubscribe mechanism to pkgconfigmodel.Reader
	//   - Implementing a Stop() method that can unregister the callback
	//   - Using weak references or callback IDs for lifecycle management
	cfg.OnUpdate(func(setting string, source pkgconfigmodel.Source, _, _ any, _ uint64) {
		// Only process config changes after Start() has been called to avoid race conditions
		// where the callback fires during initialization (before event processing begins).
		if !sub.started.Load() {
			log.Debugf("CEL service naming: ignoring config change before subscriber started")
			return
		}

		if setting == "service_discovery.enabled" || setting == "service_discovery.service_definitions" {
			log.Infof("CEL service naming: config changed (%s from %v), reloading rules", setting, source)
			if err := sub.reloadEngine(); err != nil {
				log.Warnf("CEL service naming: failed to reload engine after config change: %v", err)
			}
		}
	})

	return sub, nil
}

// Start processes workloadmeta container events (call as goroutine: go sub.Start(ctx)).
func (s *Subscriber) Start(ctx context.Context) {
	// Mark subscriber as started before processing any events or config changes.
	// This prevents the config reload callback from firing during initialization.
	s.started.Store(true)

	if s.ch == nil {
		// Subscribe to SourceAll (runtime + orchestrators). We receive our own SourceServiceDiscovery
		// events back, but idempotency checks prevent redundant evaluation.
		filter := workloadmeta.NewFilterBuilder().
			SetSource(workloadmeta.SourceAll).
			AddKind(workloadmeta.KindContainer).
			Build()

		s.ch = s.wmeta.Subscribe(subscriberName, workloadmeta.NormalPriority, filter)
		log.Debug("servicenaming subscriber subscribed to workloadmeta events (all sources)")
	}

	go s.periodicCleanup(ctx)

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
	delete(s.inputHashCache, containerID)
}

// processContainer evaluates CEL rules and updates workloadmeta with the result.
func (s *Subscriber) processContainer(ctx context.Context, container *workloadmeta.Container) {
	currentInputHash := hashContainerInput(container)

	// Skip re-evaluation if input and output unchanged (idempotency check)
	if container.CELServiceDiscovery != nil {
		s.mu.RLock()
		cachedService, exists := s.serviceNameCache[container.ID]
		cachedHash, hashExists := s.inputHashCache[container.ID]
		s.mu.RUnlock()

		if exists && hashExists {
			if container.CELServiceDiscovery.ServiceName == cachedService && currentInputHash == cachedHash {
				log.Tracef("CEL service naming: skipping re-evaluation for container %s (input and output unchanged)", container.ID)
				return
			}
		}
	}

	input := buildCELInput(container)
	engineInput := servicenaming.ToEngineInput(input)

	s.engineMu.RLock()
	currentEngine := s.engine
	s.engineMu.RUnlock()

	if currentEngine == nil {
		if container.CELServiceDiscovery != nil {
			log.Debugf("CEL service naming: disabled, clearing service discovery for container %s", container.ID)
			s.clearServiceDiscovery(container)
			s.removeFromCache(container.ID)
		}
		return
	}

	result := currentEngine.Evaluate(ctx, engineInput)

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

	if container.CELServiceDiscovery != nil {
		existing := container.CELServiceDiscovery
		if existing.ServiceName == result.ServiceName && existing.MatchedRule == result.MatchedRule {
			log.Tracef("CEL service naming: container %s already has correct service name %q", container.ID, result.ServiceName)
			s.updateCache(container.ID, result.ServiceName, currentInputHash)
			return
		}
	}

	log.Debugf("CEL service naming: container %s matched rule %q, service name: %s",
		container.ID, result.MatchedRule, result.ServiceName)

	// Update cache before push to prevent race condition if event returns immediately
	s.updateCache(container.ID, result.ServiceName, currentInputHash)

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

func (s *Subscriber) updateCache(containerID, serviceName string, inputHash uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serviceNameCache[containerID] = serviceName
	s.inputHashCache[containerID] = inputHash
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

// periodicCleanup removes stale cache entries every 10 minutes.
func (s *Subscriber) periodicCleanup(ctx context.Context) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupStaleEntries()
		}
	}
}

func (s *Subscriber) cleanupStaleEntries() {
	containers := s.wmeta.ListContainers()
	activeIDs := make(map[string]struct{}, len(containers))
	for _, c := range containers {
		activeIDs[c.ID] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	staleCount := 0
	for containerID := range s.serviceNameCache {
		if _, exists := activeIDs[containerID]; !exists {
			delete(s.serviceNameCache, containerID)
			delete(s.inputHashCache, containerID)
			staleCount++
		}
	}

	if staleCount > 0 {
		log.Debugf("CEL service naming: cleaned up %d stale container entries from cache", staleCount)
	}
}

// hashContainerInput computes a hash of CEL-relevant container metadata.
func hashContainerInput(container *workloadmeta.Container) uint64 {
	h := fnv.New64a()

	h.Write([]byte(container.Image.Name))
	h.Write([]byte(container.Image.ShortName))
	h.Write([]byte(container.Image.Tag))
	h.Write([]byte(container.Image.Registry))

	if container.Labels != nil {
		labelCount := len(container.Labels)
		if labelCount > maxLabelsForHashing {
			log.Warnf("Container %s has %d labels, only hashing first %d",
				container.ID, labelCount, maxLabelsForHashing)
		}

		keys := make([]string, 0, len(container.Labels))
		for k := range container.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		count := 0
		for _, k := range keys {
			if count >= maxLabelsForHashing {
				break
			}
			v := container.Labels[k]
			if len(v) > maxLabelValueLen {
				v = v[:maxLabelValueLen]
			}
			h.Write([]byte(k))
			h.Write([]byte(v))
			count++
		}
	}

	if container.EnvVars != nil {
		envCount := len(container.EnvVars)
		if envCount > maxEnvsForHashing {
			log.Warnf("Container %s has %d env vars, only hashing first %d",
				container.ID, envCount, maxEnvsForHashing)
		}

		keys := make([]string, 0, len(container.EnvVars))
		for k := range container.EnvVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		count := 0
		for _, k := range keys {
			if count >= maxEnvsForHashing {
				break
			}
			v := container.EnvVars[k]
			if len(v) > maxLabelValueLen {
				v = v[:maxLabelValueLen]
			}
			h.Write([]byte(k))
			h.Write([]byte(v))
			count++
		}
	}

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

	h.Write([]byte(container.Name))

	return h.Sum64()
}

// reloadEngine reloads the CEL engine from current config.
func (s *Subscriber) reloadEngine() error {
	sdConfig, err := servicenaming.LoadFromAgentConfig(s.cfg)
	if err != nil {
		return err
	}

	if !sdConfig.IsActive() {
		log.Warn("CEL service naming: config reloaded but is now inactive (disabled or no rules)")
		s.engineMu.Lock()
		s.engine = nil
		s.engineMu.Unlock()
		s.mu.Lock()
		s.serviceNameCache = make(map[string]string)
		s.inputHashCache = make(map[string]uint64)
		s.mu.Unlock()
		for _, container := range s.wmeta.ListContainers() {
			if container.CELServiceDiscovery != nil {
				s.clearServiceDiscovery(container)
			}
		}
		return nil
	}

	newEngine, err := sdConfig.CompileEngine()
	if err != nil {
		return err
	}

	if newEngine == nil {
		log.Warn("CEL service naming: config reloaded but engine is nil")
		return nil
	}

	s.engineMu.Lock()
	s.engine = newEngine
	s.engineMu.Unlock()

	log.Infof("CEL service naming: engine reloaded successfully with %d rules",
		len(sdConfig.ServiceDefinitions))

	s.mu.Lock()
	s.serviceNameCache = make(map[string]string)
	s.inputHashCache = make(map[string]uint64)
	s.mu.Unlock()
	for _, container := range s.wmeta.ListContainers() {
		s.processContainer(context.Background(), container)
	}

	return nil
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
