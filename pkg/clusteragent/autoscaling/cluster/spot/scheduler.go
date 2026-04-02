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
	"k8s.io/utils/clock"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	checkOnDemandFallbackInterval = 10 * time.Second
	rebalanceInterval             = 10 * time.Second
)

// PodHandler handles pod admission events for spot scheduling.
type PodHandler interface {
	// PodCreated is called when a pod is created via admission webhook.
	// It returns true if the pod was mutated to target a spot instance.
	PodCreated(pod *corev1.Pod) (bool, error)
	// PodDeleted is called when a pod is deleted via admission webhook.
	PodDeleted(pod *corev1.Pod)
}

// scheduler schedules eligible pods onto spot instances.
type scheduler struct {
	config      Config
	clock       clock.WithTicker
	wlm         workloadmeta.Component
	evictor     podEvictor
	patcher     workloadPatcher
	configStore workloadConfigStore
	isLeader    func() bool
	tracker     *podTracker
	controller  *workloadController
	synced      chan struct{}
}

func newScheduler(cfg Config, clk clock.WithTicker, wlm workloadmeta.Component, evictor podEvictor, patcher workloadPatcher, dynamicClient dynamic.Interface, lister podLister, isLeader func() bool) *scheduler {
	s := &scheduler{
		config:   cfg,
		clock:    clk,
		wlm:      wlm,
		evictor:  evictor,
		patcher:  patcher,
		isLeader: isLeader,
		synced:   make(chan struct{}),
	}
	defaultConfig := workloadSpotConfig{percentage: cfg.Percentage, minOnDemand: cfg.MinOnDemandReplicas}
	s.tracker = newPodTracker(clk, defaultConfig, s.getSpotConfig)
	store := newSpotConfigStore()
	s.configStore = store
	s.controller = newWorkloadController(dynamicClient, defaultConfig, store, lister, s.tracker)
	return s
}

// Start launches goroutines to track pod updates and check for on-demand fallback and returns immediately.
func (s *scheduler) Start(ctx context.Context) {
	log.Infof("Starting spot scheduler: %s", s.config)

	// Run in separate goroutines to not not delay pod updates processing.
	go s.controller.start(ctx)
	go s.trackPodUpdates(ctx)
	go s.checkOnDemandFallback(ctx)
	go s.rebalance(ctx)
}

// trackPodUpdates subscribes to workloadmeta pod events and updates the tracker.
func (s *scheduler) trackPodUpdates(ctx context.Context) {
	// Wait for the workload controller to sync before subscribing to workloadmeta events.
	// The WLM subscription delivers an initial event bundle for all existing pods filtered by spotEligibleFilter.
	// If the controller is not yet synced, spotEligibleFilter returns false for all pods
	// and existing spot-eligible pods would be missed.
	s.controller.waitSynced()

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
func (s *scheduler) checkOnDemandFallback(ctx context.Context) {
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
func (s *scheduler) rebalance(ctx context.Context) {
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
func (s *scheduler) PodCreated(pod *corev1.Pod) (bool, error) {
	unchanged := func() (bool, error) {
		return false, nil
	}

	g, ok := resolveCoreV1PodGroup(pod)
	if !ok {
		return unchanged()
	}

	cfg, ok := s.getSpotConfig(g.workload)
	if !ok {
		return unchanged()
	}

	log.Debugf("Pod created via webhook for owner %s", g.owner)

	if cfg.isDisabled(s.clock.Now()) {
		log.Debugf("Spot scheduling disabled until %v, skipping pod for %s", cfg.disabledUntil, g.owner)
		s.tracker.admitNewOnDemandPod(g)
		return unchanged()
	}

	if s.tracker.admitNewPod(g) {
		assignToSpot(pod)
		return true, nil
	}
	return unchanged()
}

// PodDeleted is called via admission webhook.
// It stops tracking the pod.
func (s *scheduler) PodDeleted(pod *corev1.Pod) {
	g, ok := resolveCoreV1PodGroup(pod)
	if !ok {
		return
	}

	if _, eligible := s.getSpotConfig(g.workload); !eligible {
		return
	}

	uid := string(pod.UID)

	log.Debugf("Pod %s (phase=%s) removed via webhook for owner %s", uid, pod.Status.Phase, g.owner)

	s.tracker.deletePod(g, uid)
}

// getSpotConfig returns the spot config for the given workload.
func (s *scheduler) getSpotConfig(w workload) (workloadSpotConfig, bool) {
	return s.configStore.getConfig(w)
}

func (s *scheduler) spotEligibleFilter(entity workloadmeta.Entity) bool {
	pod, ok := entity.(*workloadmeta.KubernetesPod)
	if !ok {
		return false
	}
	g, ok := resolveWLMPodGroup(pod)
	if !ok {
		return false
	}
	_, ok = s.getSpotConfig(g.workload)
	return ok
}

// Spot node label and taint.
// Currently Karpenter-specific.
const (
	spotNodeLabelKey   = "karpenter.sh/capacity-type"
	spotNodeLabelValue = "spot"
	spotNodeTaintKey   = "karpenter.sh/capacity-type"
	spotNodeTaintValue = "spot"
)

func assignToSpot(pod *corev1.Pod) {
	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = map[string]string{}
	}
	pod.Spec.NodeSelector[spotNodeLabelKey] = spotNodeLabelValue
	pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
		Key:      spotNodeTaintKey,
		Operator: corev1.TolerationOpEqual,
		Value:    spotNodeTaintValue,
		Effect:   corev1.TaintEffectNoSchedule,
	})

	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	pod.Labels[SpotAssignedLabel] = SpotAssignedSpot
}

// checkOnDemandFallbackOnce checks pending spot-assigned pods, disables spot scheduling and evicts pending pods for affected workloads.
func (s *scheduler) checkOnDemandFallbackOnce(ctx context.Context, now time.Time) {
	pending := s.tracker.getPendingSpotPods(now.Add(-s.config.ScheduleTimeout))
	if len(pending) == 0 {
		return
	}

	for workload, pods := range groupByWorkload(pending) {
		if err := s.disableSpotScheduling(ctx, workload, now); err != nil {
			log.Errorf("Failed to disable spot scheduling for %s: %v", workload, err)
			continue
		}
		for uid, pod := range pods {
			if err := s.evictor.evictPod(ctx, pod.workload.Namespace, pod.name, corev1.PodPending); err != nil {
				log.Errorf("Failed to evict timed-out pending spot pod %s of %s: %v", pod.name, pod.workload, err)
				continue
			}
			log.Infof("Evicted timed-out pending spot pod %s of %s for on-demand fallback", pod.name, pod.workload)
			s.tracker.deletePendingSpotPod(uid)
		}
	}
}

// groupByWorkload groups pending pods by their workload, outside the tracker lock.
func groupByWorkload(pods map[string]pendingSpotPod) map[workload]map[string]pendingSpotPod {
	result := make(map[workload]map[string]pendingSpotPod)
	for uid, pod := range pods {
		if result[pod.workload] == nil {
			result[pod.workload] = make(map[string]pendingSpotPod)
		}
		result[pod.workload][uid] = pod
	}
	return result
}

// disableSpotScheduling disables spot scheduling for the workload.
func (s *scheduler) disableSpotScheduling(ctx context.Context, workload workload, now time.Time) error {
	disabledUntil, updated := s.configStore.disable(workload, now, now.Add(s.config.FallbackDuration))
	if !updated {
		return nil
	}
	log.Infof("Disabling spot scheduling for %s until %v", workload, disabledUntil)
	return s.patcher.setDisabledUntil(ctx, workload, disabledUntil)
}
