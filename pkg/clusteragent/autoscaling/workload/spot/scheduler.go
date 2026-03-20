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
)

// Scheduler schedules eligible pods onto spot instances.
type Scheduler struct {
	config     Config
	clock      clock.WithTicker
	wlm        workloadmeta.Component
	evictor    podEvictor
	isLeader   func() bool
	tracker    *podTracker
	subscribed chan struct{}

	mu                sync.RWMutex
	spotDisabledUntil time.Time
}

// NewScheduler creates a new spot Scheduler.
func NewScheduler(cfg Config, clk clock.WithTicker, wlm workloadmeta.Component, client kubernetes.Interface, isLeader func() bool) *Scheduler {
	return newScheduler(cfg, clk, wlm, newKubePodEvictor(client), isLeader)
}

func newScheduler(cfg Config, clk clock.WithTicker, wlm workloadmeta.Component, e podEvictor, isLeader func() bool) *Scheduler {
	return &Scheduler{
		config:     cfg,
		clock:      clk,
		wlm:        wlm,
		evictor:    e,
		isLeader:   isLeader,
		tracker:    newPodTracker(clk),
		subscribed: make(chan struct{}),
	}
}

// Start launches goroutines to track pod updates and check for on-demand fallback and returns immediately.
func (s *Scheduler) Start(ctx context.Context) {
	log.Infof("Starting spot scheduler: %s", s.config)

	// Run in separate goroutines so that a slow fallback check (which may make Kubernetes API calls)
	// does not delay pod updates processing.
	go s.trackPodUpdates(ctx)
	go s.checkOnDemandFallback(ctx)
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
			}
		}
	}
}

// PodCreated is called via admission webhook.
// It decides whether a pod should be scheduled on spot and updates it accordingly.
func (s *Scheduler) PodCreated(pod *corev1.Pod) (bool, error) {
	if !s.isSpotEligible(pod) {
		return false, nil
	}

	owner, ok := resolveCoreV1PodOwner(pod)
	if !ok {
		return false, nil
	}

	log.Debugf("Pod created via webhook for owner %s", owner)

	spotPercentage, minOnDemand := s.readConfig(pod)

	disabledUntil, disabled := s.isSpotSchedulingDisabled()

	isSpot := s.tracker.admitNewPod(owner, func(total, spot int) bool {
		if disabled {
			log.Debugf("Spot scheduling disabled until %v, skipping pod for %s", disabledUntil, owner)
			return false
		}

		onDemand := total - spot
		if onDemand < minOnDemand {
			log.Debugf("Skipping pod for %s: on-demand minimum not met (%d < %d), total: %d, spot: %d", owner, onDemand, minOnDemand, total, spot)
			return false
		}

		desiredSpot := (total + 1) * spotPercentage / 100
		if spot >= desiredSpot {
			log.Debugf("Skipping pod for %s: desired spot reached (%d >= %d), total: %d", owner, spot, desiredSpot, total)
			return false
		}

		log.Debugf("Assigning pod for %s to spot (%d of desired %d spot, %d on-demand), total: %d", owner, spot, desiredSpot, onDemand, total)
		return true
	})

	if isSpot {
		assignToSpot(pod)
		return true, nil
	}
	return false, nil
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

	disabledUntil, updated := s.disableSpotScheduling(now)
	if updated {
		log.Infof("Disabling spot scheduling until %v", disabledUntil)
	}

	for uid, pod := range s.tracker.getPendingSpotPods() {
		if err := s.evictor.evictPod(ctx, pod.namespace, pod.name); err != nil {
			log.Errorf("Failed to evict timed-out pending spot pod %s/%s: %v", pod.namespace, pod.name, err)
			continue
		}
		log.Infof("Evicted timed-out pending spot pod %s/%s (owner: %s) for on-demand fallback", pod.namespace, pod.name, pod.owner)
		s.tracker.deletePendingSpotPod(uid)
	}
}

func (s *Scheduler) isSpotSchedulingDisabled() (time.Time, bool) {
	s.mu.RLock()
	spotDisabledUntil := s.spotDisabledUntil
	s.mu.RUnlock()

	return spotDisabledUntil, s.clock.Now().Before(spotDisabledUntil)
}

// reEnableSpotScheduling enables spot scheduling it was disabled and can be re-enabled and
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

// disableSpotScheduling disables spot scheduling if it is not disabled yet and
// returns timestamp until scheduling is disabled and a boolean signaling if it was just disabled.
func (s *Scheduler) disableSpotScheduling(now time.Time) (time.Time, bool) {
	disabledUntil := now.Add(s.config.DisabledInterval)
	disabledUntilUpdated := false

	s.mu.Lock()
	defer s.mu.Unlock()

	if now.Before(s.spotDisabledUntil) {
		// already disabled
		disabledUntil = s.spotDisabledUntil
	} else {
		s.spotDisabledUntil = disabledUntil
		disabledUntilUpdated = true
	}
	return disabledUntil, disabledUntilUpdated
}

// readConfig reads spot configuration from pod annotations, falling back to s.config defaults.
func (s *Scheduler) readConfig(pod *corev1.Pod) (spotPercentage int, minOnDemand int) {
	spotPercentage = s.config.Percentage
	if v := pod.Annotations[SpotPercentageAnnotation]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 100 {
			spotPercentage = n
		}
	}

	minOnDemand = s.config.MinOnDemandReplicas
	if v := pod.Annotations[SpotMinOnDemandReplicasAnnotation]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			minOnDemand = n
		}
	}

	return
}
