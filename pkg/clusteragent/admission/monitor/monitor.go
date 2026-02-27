// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package monitor watches for pods that should have been mutated by the
// admission controller but were not, which may indicate a network connectivity
// issue between the Kubernetes API server and the admission webhook.
package monitor

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const subscriberName = "admission-monitor"

// Monitor watches pods via workloadmeta and detects when pods that should have
// been mutated by the admission controller were not, indicating a possible
// network connectivity issue.
type Monitor struct {
	wmeta     workloadmeta.Component
	filter    mutatecommon.MutationFilter
	startTime time.Time
}

// NewMonitor creates a new admission monitor. The filter uses the same
// namespace/label eligibility rules as the config injection webhook.
func NewMonitor(wmeta workloadmeta.Component, datadogConfig config.Component) (*Monitor, error) {
	filter, err := mutatecommon.NewDefaultFilter(
		true,
		datadogConfig.GetStringSlice("admission_controller.enabled_namespaces"),
		datadogConfig.GetStringSlice("admission_controller.disabled_namespaces"),
	)
	if err != nil {
		return nil, err
	}

	return &Monitor{
		wmeta:     wmeta,
		filter:    filter,
		startTime: time.Now(),
	}, nil
}

// Run subscribes to workloadmeta pod events and checks each new pod to
// determine if it should have been mutated. This blocks until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	log.Info("Starting admission controller monitor")

	wlFilter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindKubernetesPod).
		SetEventType(workloadmeta.EventTypeSet).
		Build()

	ch := m.wmeta.Subscribe(subscriberName, workloadmeta.NormalPriority, wlFilter)
	defer m.wmeta.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping admission controller monitor")
			return
		case eventBundle, more := <-ch:
			eventBundle.Acknowledge()
			if !more {
				return
			}
			for _, event := range eventBundle.Events {
				m.handleEvent(event)
			}
		}
	}
}

func (m *Monitor) handleEvent(event workloadmeta.Event) {
	pod, ok := event.Entity.(*workloadmeta.KubernetesPod)
	if !ok {
		return
	}

	// Only inspect pods created after the monitor started to avoid
	// flagging pre-existing pods that were created before the
	// annotation marker was introduced.
	if pod.CreationTimestamp.Before(m.startTime) {
		return
	}

	if !m.shouldHaveBeenMutated(pod) {
		return
	}

	if _, mutated := pod.Annotations[admcommon.MutatedByWebhookAnnotationKey]; mutated {
		return
	}

	log.Errorf(
		"Pod %s/%s should have been mutated by the admission controller but was not. "+
			"This may indicate a network connectivity issue between the Kubernetes API server and the cluster agent admission webhook.",
		pod.Namespace, pod.Name,
	)
}

// shouldHaveBeenMutated constructs a minimal corev1.Pod from the workloadmeta
// entity and runs it through the same mutation filter used by the admission
// controller webhooks.
func (m *Monitor) shouldHaveBeenMutated(pod *workloadmeta.KubernetesPod) bool {
	k8sPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Labels:    pod.Labels,
		},
	}
	return m.filter.ShouldMutatePod(k8sPod)
}
