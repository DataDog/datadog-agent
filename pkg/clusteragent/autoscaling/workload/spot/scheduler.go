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
	"k8s.io/client-go/dynamic"
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
	rollout    rollout
	isLeader   func() bool
	tracker    *podTracker
	subscribed chan struct{}

	mu                sync.RWMutex
	spotDisabledUntil time.Time
}

// NewScheduler creates a new spot Scheduler.
func NewScheduler(cfg Config, clk clock.WithTicker, wlm workloadmeta.Component, client dynamic.Interface, isLeader func() bool) *Scheduler {
	return newScheduler(cfg, clk, wlm, newKubeRollout(client), isLeader)
}

func newScheduler(cfg Config, clk clock.WithTicker, wlm workloadmeta.Component, r rollout, isLeader func() bool) *Scheduler {
	return &Scheduler{
		config:     cfg,
		clock:      clk,
		wlm:        wlm,
		rollout:    r,
		isLeader:   isLeader,
		tracker:    newPodTracker(clk),
		subscribed: make(chan struct{}),
	}
}

// WaitSubscribed returns a channel that is closed once Run has subscribed to workloadmeta events.
func (s *Scheduler) WaitSubscribed() <-chan struct{} {
	return s.subscribed
}

// Config returns the scheduler configuration.
func (s *Scheduler) Config() Config {
	return s.config
}

// Run subscribes to workloadmeta pod events and periodically checks for on-demand fallback.
func (s *Scheduler) Run(ctx context.Context) {
	log.Infof("Starting spot scheduler: %s", s.config)

	filter := workloadmeta.NewFilterBuilder().AddKind(workloadmeta.KindKubernetesPod).Build()
	ch := s.wlm.Subscribe("spot-scheduler", workloadmeta.NormalPriority, filter)
	close(s.subscribed)
	defer s.wlm.Unsubscribe(ch)

	ticker := s.clock.NewTicker(checkOnDemandFallbackInterval)
	defer ticker.Stop()

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
					s.tracker.removed(pod)
				}
			}
			eventBundle.Acknowledge()
		case now := <-ticker.C():
			if s.isLeader() {
				s.checkOnDemandFallback(ctx, now)
			}
		}
	}
}

// ApplyRecommendations decides whether a pod should be scheduled on spot and updates it accordingly.
func (s *Scheduler) ApplyRecommendations(pod *corev1.Pod) (bool, error) {
	if !s.isSpotEligible(pod) {
		return false, nil
	}

	owner, ok := resolveCoreV1PodOwner(pod)
	if !ok {
		return false, nil
	}

	spotPercentage, minOnDemand := s.readConfig(pod)

	disabledUntil, disabled := s.IsSpotSchedulingDisabled()

	isSpot := s.tracker.admitNewPod(owner, func(total, spot int) bool {
		if disabled {
			log.Debugf("Spot scheduling disabled until %v, skipping pod for %s", disabledUntil, owner)
			return false
		}

		onDemand := total - spot
		if onDemand < minOnDemand {
			log.Debugf("Skipping pod for %s: on-demand minimum not met (%d < %d)", owner, onDemand, minOnDemand)
			return false
		}

		desiredSpot := (total + 1) * spotPercentage / 100
		if spot >= desiredSpot {
			log.Debugf("Skipping pod for %s: desired spot reached (%d >= %d)", owner, spot, desiredSpot)
			return false
		}

		log.Debugf("Assigning pod for %s to spot (%d of desired %d spot, %d on-demand)", owner, spot, desiredSpot, onDemand)
		return true
	})

	if isSpot {
		assignToSpot(pod)
		return true, nil
	}
	return false, nil
}

func (s *Scheduler) isSpotEligible(pod *corev1.Pod) bool {
	return pod.Annotations[SpotEnabledAnnotation] == "true"
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

	// podTracker needs to know pod assignment via a label.
	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	pod.Labels[SpotAssignedLabel] = "true"
}

// checkOnDemandFallback checks pending spot-assigned pods, disables spot scheduling and triggers rollout for pending workloads if needed.
func (s *Scheduler) checkOnDemandFallback(ctx context.Context, now time.Time) {
	if s.reEnableSpotScheduling(now) {
		log.Infof("Spot scheduling re-enabled")
	}

	pendingSpotPodsByRolloutOwner := s.tracker.getPendingSpotPods(now.Add(-s.config.ScheduleTimeout))
	if len(pendingSpotPodsByRolloutOwner) == 0 {
		return
	}

	disabledUntil, updated := s.disableSpotScheduling(now)
	if updated {
		log.Infof("Disabling spot scheduling until %v", disabledUntil)
	}

	for owner, pods := range pendingSpotPodsByRolloutOwner {
		// Restart workload.
		// New pods will be on-demand since spot scheduling is disabled at this point.
		// Use disabledUntil timestamp for no-op patch in case owner was already patched in the previous cycle and
		// pending pods have updated since then.
		updated, err := s.rollout.restart(ctx, owner, disabledUntil)
		if err != nil {
			log.Errorf("Failed to trigger rollout restart for %s: %v", owner, err)
			continue
		}
		if updated {
			log.Infof("%s has %d timed-out spot pod(s), triggered rollout restart for on-demand fallback", owner, len(pods))
		} else {
			log.Debugf("Rollout already restarted for %s, skipping", owner)
		}
		s.tracker.removePendingSpotPods(pods)
	}
}

// IsSpotSchedulingDisabled return true if spot scheduling is disabled and a timestamp until it is disabled.
func (s *Scheduler) IsSpotSchedulingDisabled() (time.Time, bool) {
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
