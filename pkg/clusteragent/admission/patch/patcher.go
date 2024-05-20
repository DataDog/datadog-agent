// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package patch implements the patching of Kubernetes deployments.
package patch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/telemetry"
	k8sutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	jsonpatch "github.com/evanphx/json-patch"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type patcher struct {
	k8sClient          kubernetes.Interface
	isLeader           func() bool
	deploymentsQueue   chan Request
	telemetryCollector telemetry.TelemetryCollector
	clusterName        string
	// Map of RC IDs to the list of namespaces in scope for the remote config;
	// to be used for removing APM Instrumentation for hard deleted RC configurations
	configIDToNamespaces map[string]*[]string
}

func newPatcher(k8sClient kubernetes.Interface, isLeaderFunc func() bool, telemetryCollector telemetry.TelemetryCollector, pp patchProvider, clusterName string) *patcher {
	return &patcher{
		k8sClient:            k8sClient,
		isLeader:             isLeaderFunc,
		deploymentsQueue:     pp.subscribe(KindCluster),
		telemetryCollector:   telemetryCollector,
		clusterName:          clusterName,
		configIDToNamespaces: make(map[string]*[]string),
	}
}

func (p *patcher) start(stopCh <-chan struct{}) {
	for {
		select {
		case req := <-p.deploymentsQueue:
			metrics.PatchAttempts.Inc()
			if err := p.patchNamespaces(req); err != nil {
				metrics.PatchErrors.Inc()
				log.Error(err.Error())
			}
		case <-stopCh:
			log.Info("Shutting down patcher")
			return
		}
	}
}

func (p *patcher) patchNamespaces(req Request) error {
	if !p.isLeader() {
		log.Debug("Not leader, skipping")
		return nil
	}

	var namespaces []string
	if req.Action == DeleteConfig {
		// If the action is to hard delete existing RC configuration, get the list of namespaces that were instrumented by the RC configuration
		namespaces = *p.configIDToNamespaces[req.ID]
		defer delete(p.configIDToNamespaces, req.ID)
	} else {
		// If the action is to enable/disable RC configuration, get the list of namespaces from K8sTarget
		namespaces = p.getNamespacesToInstrument(req.K8sTarget.ClusterTargets)
		p.configIDToNamespaces[req.ID] = &namespaces
	}

	for _, ns := range namespaces {
		namespace, err := p.k8sClient.CoreV1().Namespaces().Get(context.TODO(), ns, metav1.GetOptions{})
		if err != nil {
			return err
		}
		oldObj, err := json.Marshal(namespace)
		if err != nil {
			return fmt.Errorf("failed to encode object: %v", err)
		}
		if namespace.ObjectMeta.Labels == nil {
			namespace.ObjectMeta.Labels = make(map[string]string)
		}

		switch req.Action {
		case EnableConfig:
			enableConfig(namespace, req)
		case DisableConfig:
			disableConfig(namespace)
		case DeleteConfig:
			deleteConfig(namespace, req.ID)
		default:
			return fmt.Errorf("unknown action %q", req.Action)
		}

		newObj, err := json.Marshal(namespace)
		if err != nil {
			return fmt.Errorf("failed to encode object: %v", err)
		}
		patch, err := jsonpatch.CreateMergePatch(oldObj, newObj)
		if err != nil {
			return fmt.Errorf("failed to build the JSON patch: %v", err)
		}

		if _, err = p.k8sClient.CoreV1().Namespaces().Patch(context.TODO(), ns, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
			p.telemetryCollector.SendRemoteConfigPatchEvent(req.getApmRemoteConfigEvent(err, telemetry.FailedToMutateConfig))
			return err
		}
	}

	return nil
}

// getNamespacesToInstrument returns the list of namespaces that will be affected by RC configuration
func (p *patcher) getNamespacesToInstrument(targets []K8sClusterTarget) []string {

	enabledNamespaces := []string{}

	for _, clusterTarget := range targets {
		if clusterTarget.Enabled != nil && *clusterTarget.Enabled {
			if clusterTarget.EnabledNamespaces == nil || len(*clusterTarget.EnabledNamespaces) == 0 {
				// enable APM Instrumentation in the cluster
				nsList, err := p.k8sClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					log.Errorf("Remote Enablement: could not get list of namespaces in the cluster %s", p.clusterName)
					continue
				}
				for _, ns := range nsList.Items {
					enabledNamespaces = append(enabledNamespaces, ns.GetName())
				}
			} else {
				// enable APM Instrumentation in specific namespaces
				enabledNamespaces = *clusterTarget.EnabledNamespaces
			}

		} else {
			log.Errorf("Remote Enablement: disabling APM instrumentation via RC is not supported")
			continue
		}
	}
	return enabledNamespaces
}

// enableConfig adds APM Instrumention label and annotations to the namespace
func enableConfig(ns *v1.Namespace, req Request) {
	oldLabel, labelOk := ns.ObjectMeta.Labels[k8sutil.RcLabelKey]
	oldID, annotationOk := ns.ObjectMeta.Annotations[k8sutil.RcIDAnnotKey]

	if labelOk && oldLabel == "true" && annotationOk && oldID != req.ID {
		// If the namespace is already instrumented by another RC configuration, ignore new enable config request
		log.Debugf("APM Instrumentation has been enabled by config ID %s. Ignoring config ID %s", oldID, req.ID)
		return
	} else if labelOk && oldLabel == "false" && annotationOk && oldID != req.ID {
		// If the namespace instrumentation was soft disabled, don't allow re-instrumenting it as a part of the new scope
		log.Errorf("APM Instrumentation is turned off by disabled RC config %s", oldID)
		return
	}

	if ns.ObjectMeta.Labels == nil {
		ns.ObjectMeta.Labels = make(map[string]string)
	}
	ns.ObjectMeta.Labels[k8sutil.RcLabelKey] = "true"

	if ns.ObjectMeta.Annotations == nil {
		ns.ObjectMeta.Annotations = make(map[string]string)
	}

	ns.ObjectMeta.Annotations[k8sutil.RcIDAnnotKey] = req.ID
	ns.ObjectMeta.Annotations[k8sutil.RcRevisionAnnotKey] = fmt.Sprint(req.Revision)
}

// disableConfig removes APM Instrumention label from the namespace
func disableConfig(ns *v1.Namespace) {
	rcIDLabelVal, ok := ns.ObjectMeta.Labels[k8sutil.RcLabelKey]
	if !ok {
		log.Errorf("APM Instrumentation cannot be disabled in namespace %s because the namespace is missing RC label")
	}
	if rcIDLabelVal == "true" {
		ns.ObjectMeta.Labels[k8sutil.RcLabelKey] = "false"
	}
}

// deleteConfig removes APM Instrumention label and annotations from the namespace
func deleteConfig(ns *v1.Namespace, reqID string) {
	delete(ns.ObjectMeta.Labels, k8sutil.RcLabelKey)
	if len(ns.ObjectMeta.Labels) == 0 {
		ns.Labels = nil
	}

	if id, ok := ns.ObjectMeta.Annotations[k8sutil.RcIDAnnotKey]; ok {
		if id != reqID {
			log.Errorf("APM Instrumentation cannot be deleted for provided RC ID")
			return
		}
		delete(ns.ObjectMeta.Annotations, k8sutil.RcIDAnnotKey)
		delete(ns.ObjectMeta.Annotations, k8sutil.RcRevisionAnnotKey)
		if len(ns.ObjectMeta.Annotations) == 0 {
			ns.ObjectMeta.Annotations = nil
		}
	} else {
		log.Infof("Missing RC annotation on the namespace affected by RC configuration deletion")
	}
}
