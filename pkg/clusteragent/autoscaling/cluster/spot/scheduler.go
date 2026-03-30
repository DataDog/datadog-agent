// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
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
	config      Config
	clock       clock.WithTicker
	wlm         workloadmeta.Component
	evictor     podEvictor
	patcher     workloadPatcher
	configStore workloadConfigStore
	isLeader    func() bool
	tracker     *podTracker
	synced      chan struct{}
}

// NewScheduler creates a new spot Scheduler.
func NewScheduler(cfg Config, clk clock.WithTicker, wlm workloadmeta.Component, client k8sclient.Interface, dynamicClient dynamic.Interface, isLeader func() bool) *Scheduler {
	return newScheduler(
		cfg, clk, wlm,
		newKubePodEvictor(client),
		newKubeWorkloadPatcher(dynamicClient),
		dynamicClient,
		isLeader)
}

func newScheduler(cfg Config, clk clock.WithTicker, wlm workloadmeta.Component, evictor podEvictor, patcher workloadPatcher, dynamicClient dynamic.Interface, isLeader func() bool) *Scheduler {
	s := &Scheduler{
		config:      cfg,
		clock:       clk,
		wlm:         wlm,
		evictor:     evictor,
		patcher:     patcher,
		configStore: newKubeWorkloadConfigStore(dynamicClient, cfg),
		isLeader:    isLeader,
		synced:      make(chan struct{}),
	}
	s.tracker = newPodTracker(clk, spotConfig{percentage: cfg.Percentage, minOnDemand: cfg.MinOnDemandReplicas}, s.getSpotConfig)
	return s
}

// Start launches goroutines to track pod updates and check for on-demand fallback and returns immediately.
func (s *Scheduler) Start(ctx context.Context) {
	log.Infof("Starting spot scheduler: %s", s.config)

	// Run in separate goroutines to not not delay pod updates processing.
	go s.configStore.run(ctx)
	go s.trackPodUpdates(ctx)
	go s.checkOnDemandFallback(ctx)
	go s.rebalance(ctx)
}

// trackPodUpdates subscribes to workloadmeta pod events and updates the tracker.
func (s *Scheduler) trackPodUpdates(ctx context.Context) {
	// Wait for the config store to sync before subscribing to workloadmeta events.
	// The WLM subscription delivers an initial event bundle for all existing pods filtered by spotEligibleFilter.
	// If the config store is not yet synced, spotEligibleFilter returns false for all pods
	// and existing spot-eligible pods would be missed.
	s.configStore.waitSynced()

	filter := workloadmeta.NewFilterBuilder().AddKindWithEntityFilter(workloadmeta.KindKubernetesPod, s.spotEligibleFilter).Build()
	ch := s.wlm.Subscribe("spot-scheduler", workloadmeta.NormalPriority, filter)
	close(s.synced)
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

	owner, ok := resolveCoreV1PodOwner(pod)
	if !ok {
		return unchanged()
	}

	cfg, ok := s.getSpotConfig(owner)
	if !ok {
		return unchanged()
	}

	log.Debugf("Pod created via webhook for owner %s", owner)

	if cfg.isDisabled(s.clock.Now()) {
		log.Debugf("Spot scheduling disabled until %v, skipping pod for %s", cfg.disabledUntil, owner)
		s.tracker.admitNewOnDemandPod(owner)
		return unchanged()
	}

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

// getSpotConfig returns the spot config for the given owner.
func (s *Scheduler) getSpotConfig(owner podOwner) (spotConfig, bool) {
	workload, ok := resolveOwnerWorkload(owner)
	if !ok {
		return spotConfig{}, false
	}
	return s.configStore.getConfig(workload)
}

func (s *Scheduler) isSpotEligible(pod *corev1.Pod) bool {
	owner, hasOwner := resolveCoreV1PodOwner(pod)
	if !hasOwner {
		return false
	}
	_, ok := s.getSpotConfig(owner)
	return ok
}

func (s *Scheduler) spotEligibleFilter(entity workloadmeta.Entity) bool {
	pod, ok := entity.(*workloadmeta.KubernetesPod)
	if !ok {
		return false
	}
	owner, hasOwner := resolveWLMPodOwner(pod)
	if !hasOwner {
		return false
	}
	_, ok = s.getSpotConfig(owner)
	return ok
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

// checkOnDemandFallbackOnce checks pending spot-assigned pods, disables spot scheduling and evicts pending pods for affected workloads.
func (s *Scheduler) checkOnDemandFallbackOnce(ctx context.Context, now time.Time) {
	pending := s.tracker.getPendingSpotPods(now.Add(-s.config.ScheduleTimeout))
	if len(pending) == 0 {
		return
	}

	byWorkload, orphaned := groupByWorkload(pending)
	for workload, pods := range byWorkload {
		if err := s.disableSpotScheduling(ctx, workload, now); err != nil {
			log.Errorf("Failed to disable spot scheduling for %s: %v", workload, err)
			continue
		}
		for uid, pod := range pods {
			if err := s.evictor.evictPod(ctx, pod.owner.Namespace, pod.name, corev1.PodPending); err != nil {
				log.Errorf("Failed to evict timed-out pending spot pod %s/%s: %v", pod.owner.Namespace, pod.name, err)
				continue
			}
			log.Infof("Evicted timed-out pending spot pod %s/%s for on-demand fallback", pod.owner.Namespace, pod.name)
			s.tracker.deletePendingSpotPod(uid)
		}
	}

	// There should not be any orphaned pods due to admission check
	for uid, pod := range orphaned {
		log.Warnf("Cannot resolve workload for pending spot pod %s/%s (owner %s, uid %s)", pod.owner.Namespace, pod.name, pod.owner, uid)
	}
}

// groupByWorkload resolves each pending pod's owner to its top-level workload and groups by it.
// It returns the grouped pods and a map of pods whose owner could not be resolved (keyed by pod UID).
func groupByWorkload(pods map[string]pendingSpotPod) (map[workload]map[string]pendingSpotPod, map[string]pendingSpotPod) {
	result := make(map[workload]map[string]pendingSpotPod)
	var orphaned map[string]pendingSpotPod
	for uid, pod := range pods {
		w, ok := resolveOwnerWorkload(pod.owner)
		if !ok {
			// Should not happen but collect them for logging.
			if orphaned == nil {
				orphaned = make(map[string]pendingSpotPod)
			}
			orphaned[uid] = pod
			continue
		}
		if _, ok := result[w]; !ok {
			result[w] = make(map[string]pendingSpotPod)
		}
		result[w][uid] = pod
	}
	return result, orphaned
}

// disableSpotScheduling disables spot scheduling for the workload.
func (s *Scheduler) disableSpotScheduling(ctx context.Context, workload workload, now time.Time) error {
	disabledUntil, updated := s.configStore.disable(workload, now, now.Add(s.config.FallbackDuration))
	if !updated {
		return nil
	}
	log.Infof("Disabling spot scheduling for %s until %v", workload, disabledUntil)
	return s.patcher.setDisabledUntil(ctx, workload, disabledUntil)
}
