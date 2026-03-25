// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/clock"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	checkOnDemandFallbackInterval = 10 * time.Second
	rebalanceInterval             = 10 * time.Second
)

// Scheduler schedules eligible pods onto spot instances.
type Scheduler struct {
	config        Config
	clock         clock.WithTicker
	wlm           workloadmeta.Component
	evictor       podEvictor
	fallbackStore fallbackStore
	isLeader      func() bool
	tracker       *podTracker
	subscribed    chan struct{}

	mu                sync.RWMutex
	spotDisabledUntil time.Time
}

// NewScheduler creates a new spot Scheduler.
func NewScheduler(cfg Config, clk clock.WithTicker, wlm workloadmeta.Component, client kubernetes.Interface, namespace string, isLeader func() bool) *Scheduler {
	return newScheduler(cfg, clk, wlm, newKubePodEvictor(client), newConfigMapFallbackStore(client, namespace), isLeader)
}

func newScheduler(cfg Config, clk clock.WithTicker, wlm workloadmeta.Component, evictor podEvictor, store fallbackStore, isLeader func() bool) *Scheduler {
	return &Scheduler{
		config:        cfg,
		clock:         clk,
		wlm:           wlm,
		evictor:       evictor,
		fallbackStore: store,
		isLeader:      isLeader,
		tracker:       newPodTracker(clk, spotConfig{percentage: cfg.Percentage, minOnDemand: cfg.MinOnDemandReplicas}),
		subscribed:    make(chan struct{}),
	}
}

// Start launches goroutines to track pod updates and check for on-demand fallback and returns immediately.
func (s *Scheduler) Start(ctx context.Context) {
	log.Infof("Starting spot scheduler: %s", s.config)

	// Run in separate goroutines so that a slow fallback check (which may make Kubernetes API calls)
	// does not delay pod updates processing.
	go s.trackPodUpdates(ctx)
	go s.checkOnDemandFallback(ctx)
	go s.rebalance(ctx)
}

// trackPodUpdates subscribes to workloadmeta pod events and updates the tracker.
func (s *Scheduler) trackPodUpdates(ctx context.Context) {
	filter := workloadmeta.NewFilterBuilder().AddKindWithEntityFilter(workloadmeta.KindKubernetesPod, s.spotEligibleFilter).Build()
	ch := s.wlm.Subscribe("spot-scheduler", workloadmeta.NormalPriority, filter)
	close(s.subscribed)
	defer s.wlm.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			log.Debug("Stopping")
			return
		case eventBundle, more := <-ch:
			if !more {
				eventBundle.Acknowledge()
				log.Debug("Stopping")
				return
			}
			for _, event := range eventBundle.Events {
				pod, _ := event.Entity.(*workloadmeta.KubernetesPod)
				switch event.Type {
				case workloadmeta.EventTypeSet:
					s.tracker.addedOrUpdated(pod)
				case workloadmeta.EventTypeUnset:
					s.tracker.deleted(pod)
				}
			}
			eventBundle.Acknowledge()
		}
	}
}

// checkOnDemandFallback periodically checks for pending spot pods and triggers on-demand fallback if needed.
func (s *Scheduler) checkOnDemandFallback(ctx context.Context) {
	ticker := s.clock.NewTicker(checkOnDemandFallbackInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C():
			if s.isLeader() {
				s.checkOnDemandFallbackOnce(ctx, now)
			} else {
				s.syncOnDemandFallbackState(ctx)
			}
		}
	}
}

// rebalance periodically evicts pods that are over the desired spot/on-demand ratio,
// allowing the owning controller to recreate them with the correct scheduling.
func (s *Scheduler) rebalance(ctx context.Context) {
	ticker := s.clock.NewTicker(rebalanceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			if !s.isLeader() {
				continue
			}
			_, disabled := s.isSpotSchedulingDisabled()
			if disabled {
				continue
			}
			uid, name, namespace := s.tracker.getPodToDelete(s.config.RebalanceStabilizationPeriod)
			if uid == "" {
				continue
			}
			if err := s.evictor.evictPod(ctx, namespace, name, ""); err != nil {
				log.Errorf("Failed to evict pod %s/%s for rebalancing: %v", namespace, name, err)
				continue
			}
			log.Infof("Evicted pod %s/%s for spot rebalancing", namespace, name)
		}
	}
}

// PodCreated is called via admission webhook.
// It decides whether a pod should be scheduled on spot and updates it accordingly.
// On-demand pods are left unchanged for resilience: if the webhook is unavailable,
// pods are still scheduled normally and no other component depend on modifications.
func (s *Scheduler) PodCreated(pod *corev1.Pod) (bool, error) {
	unchanged := func() (bool, error) {
		return false, nil
	}

	if !s.isSpotEligible(pod) {
		return unchanged()
	}

	owner, ok := resolveCoreV1PodOwner(pod)
	if !ok {
		return unchanged()
	}

	log.Debugf("Pod created via webhook for owner %s", owner)

	disabledUntil, disabled := s.isSpotSchedulingDisabled()
	if disabled {
		log.Debugf("Spot scheduling disabled until %v, skipping pod for %s", disabledUntil, owner)
		s.tracker.admitNewOnDemandPod(owner)
		return unchanged()
	}

	// For now update config from pod annotations.
	// TODO: update config from owner annotations asynchronously.
	s.tracker.updateConfig(owner, s.readConfig(pod))

	if s.tracker.admitNewPod(owner) {
		assignToSpot(pod)
		return true, nil
	}
	return unchanged()
}

// PodDeleted is called via admission webhook.
// It stops tracking the pod.
func (s *Scheduler) PodDeleted(pod *corev1.Pod) {
	if !s.isSpotEligible(pod) {
		return
	}

	owner, ok := resolveCoreV1PodOwner(pod)
	if !ok {
		return
	}
	uid := string(pod.UID)

	log.Debugf("Pod %s (phase=%s) removed via webhook for owner %s", uid, pod.Status.Phase, owner)

	s.tracker.deletePod(owner, uid)
}

func (s *Scheduler) isSpotEligible(pod *corev1.Pod) bool {
	return pod.Annotations[SpotEnabledAnnotation] == "true"
}

func (s *Scheduler) spotEligibleFilter(entity workloadmeta.Entity) bool {
	pod, ok := entity.(*workloadmeta.KubernetesPod)
	return ok && pod.Annotations[SpotEnabledAnnotation] == "true"
}

func assignToSpot(pod *corev1.Pod) {
	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = map[string]string{}
	}
	pod.Spec.NodeSelector[KarpenterCapacityTypeLabel] = KarpenterCapacityTypeSpot
	pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
		Key:      KarpenterCapacityTypeLabel,
		Operator: corev1.TolerationOpEqual,
		Value:    KarpenterCapacityTypeSpot,
		Effect:   corev1.TaintEffectNoSchedule,
	})

	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	pod.Labels[SpotAssignedLabel] = SpotAssignedSpot
}

// checkOnDemandFallbackOnce checks pending spot-assigned pods, disables spot scheduling and evicts timed-out pods if needed.
func (s *Scheduler) checkOnDemandFallbackOnce(ctx context.Context, now time.Time) {
	if s.reEnableSpotScheduling(now) {
		log.Infof("Spot scheduling re-enabled")
	}

	if !s.tracker.hasPendingSpotPods(now.Add(-s.config.ScheduleTimeout)) {
		return
	}

	disabledUntil, updated := s.disableSpotScheduling(ctx, now)
	if updated {
		log.Infof("Disabling spot scheduling until %v", disabledUntil)
	}

	for uid, pod := range s.tracker.getPendingSpotPods() {
		if err := s.evictor.evictPod(ctx, pod.namespace, pod.name, corev1.PodPending); err != nil {
			log.Errorf("Failed to evict timed-out pending spot pod %s/%s: %v", pod.namespace, pod.name, err)
			continue
		}
		log.Infof("Evicted timed-out pending spot pod %s/%s (owner: %s) for on-demand fallback", pod.namespace, pod.name, pod.owner)
		s.tracker.deletePendingSpotPod(uid)
	}
}

// syncFallbackState reads the disabled-until timestamp from the store and updates in-memory state.
func (s *Scheduler) syncOnDemandFallbackState(ctx context.Context) {
	until, err := s.fallbackStore.read(ctx)
	if err != nil {
		log.Errorf("Failed to sync spot fallback state: %v", err)
		return
	}

	updated := false
	s.mu.Lock()
	if !until.IsZero() && until.After(s.spotDisabledUntil) {
		s.spotDisabledUntil = until
		updated = true
	}
	s.mu.Unlock()

	if updated {
		log.Infof("Spot scheduling disabled until %v", until)
	}
}

func (s *Scheduler) isSpotSchedulingDisabled() (time.Time, bool) {
	s.mu.RLock()
	spotDisabledUntil := s.spotDisabledUntil
	s.mu.RUnlock()

	return spotDisabledUntil, s.clock.Now().Before(spotDisabledUntil)
}

// reEnableSpotScheduling enables spot scheduling if it was disabled and can be re-enabled and
// returns true if scheduling was re-enabled.
func (s *Scheduler) reEnableSpotScheduling(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	reEnabled := !s.spotDisabledUntil.IsZero() && now.After(s.spotDisabledUntil)
	if reEnabled {
		s.spotDisabledUntil = time.Time{}
	}
	return reEnabled
}

// disableSpotScheduling disables spot scheduling if it is not disabled yet, persists the timestamp
// to the store, and returns timestamp until scheduling is disabled and a boolean signaling if it was just disabled.
func (s *Scheduler) disableSpotScheduling(ctx context.Context, now time.Time) (time.Time, bool) {
	disabledUntil := now.Add(s.config.FallbackDuration)
	updated := false

	s.mu.Lock()
	if now.Before(s.spotDisabledUntil) {
		// already disabled
		disabledUntil = s.spotDisabledUntil
	} else {
		s.spotDisabledUntil = disabledUntil
		updated = true
	}
	s.mu.Unlock()

	if updated {
		if err := s.fallbackStore.store(ctx, disabledUntil); err != nil {
			log.Errorf("Failed to persist spot fallback state: %v", err)
		}
	}
	return disabledUntil, updated
}

// readConfig reads spot configuration from pod annotations, falling back to s.config defaults.
func (s *Scheduler) readConfig(pod *corev1.Pod) spotConfig {
	config := spotConfig{
		percentage:  s.config.Percentage,
		minOnDemand: s.config.MinOnDemandReplicas,
	}

	if v := pod.Annotations[SpotPercentageAnnotation]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 100 {
			config.percentage = n
		}
	}

	if v := pod.Annotations[SpotMinOnDemandReplicasAnnotation]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			config.minOnDemand = n
		}
	}

	return config
}
