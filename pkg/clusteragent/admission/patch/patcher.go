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
}

func newPatcher(k8sClient kubernetes.Interface, isLeaderFunc func() bool, telemetryCollector telemetry.TelemetryCollector, pp patchProvider, clusterName string) *patcher {
	return &patcher{
		k8sClient:          k8sClient,
		isLeader:           isLeaderFunc,
		deploymentsQueue:   pp.subscribe(KindDeployment),
		telemetryCollector: telemetryCollector,
		clusterName:        clusterName,
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

	enabledNamespaces := []string{}

	for _, clusterTarget := range req.K8sTargetV2.ClusterTargets {

		if clusterTarget.ClusterName != p.clusterName {
			log.Debugf("Remote Enablement: ignoring cluster target with cluster name %s. Current cluster name is %s", clusterTarget.ClusterName, p.clusterName)
			continue
		}

		if clusterTarget.Enabled != nil && *clusterTarget.Enabled {
			if clusterTarget.EnabledNamespaces == nil || len(*clusterTarget.EnabledNamespaces) == 0 {
				// enable APM Instrumentation in the cluster
				nsList, err := p.k8sClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					log.Errorf("Remote Enablement: could not get list of namespaces in the cluster %s", p.clusterName)
					// TODO: add telemetry and error apply status
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
			log.Infof("Remote Enablement: to disable APM instrumentation, delete Remote Enablement rule")
			continue
		}
	}

	revision := fmt.Sprint(req.Revision)

	for _, ns := range enabledNamespaces {
		namespace, err := p.k8sClient.CoreV1().Namespaces().Get(context.TODO(), ns, metav1.GetOptions{})
		if err != nil {
			return err
		}
		oldObj, err := json.Marshal(ns)
		if err != nil {
			return fmt.Errorf("failed to encode object: %v", err)
		}
		if namespace.Annotations == nil {
			namespace.Annotations = make(map[string]string)
		}
		if namespace.Annotations[k8sutil.RcIDAnnotKey] == req.ID && namespace.Annotations[k8sutil.RcRevisionAnnotKey] == revision {
			log.Infof("Remote Config ID %q with revision %q has already been applied to namespace %s, skipping", req.ID, revision, "")
			return nil
		}

		switch req.Action {
		case EnableConfig:
			enableConfig(namespace, req)
		case DisableConfig:
			disableConfig(namespace, req)
		default:
			return fmt.Errorf("unknown action %q", req.Action)
		}

		newObj, err := json.Marshal(ns)
		if err != nil {
			return fmt.Errorf("failed to encode object: %v", err)
		}
		patch, err := jsonpatch.CreateMergePatch(oldObj, newObj)
		if err != nil {
			return fmt.Errorf("failed to build the JSON patch: %v", err)
		}

		if _, err = p.k8sClient.CoreV1().Namespaces().Patch(context.TODO(), ns, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
			//p.telemetryCollector.SendRemoteConfigMutateEvent(req.getApmRemoteConfigEvent(err, telemetry.FailedToMutateConfig))
			return err
		}

	}

	// ns.Annotations[k8sutil.RcIDAnnotKey] = req.ID
	// ns.Annotations[k8sutil.RcRevisionAnnotKey] = revision

	//log.Infof("Patching %s with patch %s", req.K8sTarget, string(patch))

	//p.telemetryCollector.SendRemoteConfigMutateEvent(req.getApmRemoteConfigEvent(nil, telemetry.Success))
	//metrics.PatchCompleted.Inc()

	return nil
}

func enableConfig(ns *v1.Namespace, req Request) {
	ns.Annotations[k8sutil.RcIDAnnotKey] = req.ID
	ns.Annotations[k8sutil.RcRevisionAnnotKey] = fmt.Sprint(req.Revision)
}

func disableConfig(ns *v1.Namespace, req Request) {
	delete(ns.Annotations, k8sutil.RcIDAnnotKey)
	delete(ns.Annotations, k8sutil.RcRevisionAnnotKey)
}
