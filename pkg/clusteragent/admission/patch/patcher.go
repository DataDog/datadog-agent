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
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	workloadpatcher "github.com/DataDog/datadog-agent/pkg/clusteragent/patcher"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/telemetry"
	k8sutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type patcher struct {
	k8sClient          kubernetes.Interface
	patchClient        *workloadpatcher.Patcher
	isLeader           func() bool
	deploymentsQueue   chan Request
	telemetryCollector telemetry.TelemetryCollector
}

func newPatcher(k8sClient kubernetes.Interface, dynamicClient dynamic.Interface, isLeaderFunc func() bool, telemetryCollector telemetry.TelemetryCollector, pp patchProvider) *patcher {
	wp := workloadpatcher.NewPatcher(dynamicClient, nil) // leader check self managed
	return &patcher{
		k8sClient:          k8sClient,
		patchClient:        wp,
		isLeader:           isLeaderFunc,
		deploymentsQueue:   pp.subscribe(KindDeployment),
		telemetryCollector: telemetryCollector,
	}
}

func (p *patcher) start(stopCh <-chan struct{}) {
	for {
		select {
		case req := <-p.deploymentsQueue:
			metrics.PatchAttempts.Inc()
			if err := p.patchDeployment(req); err != nil {
				metrics.PatchErrors.Inc()
				log.Error(err.Error())
			}
		case <-stopCh:
			log.Info("Shutting down patcher")
			return
		}
	}
}

// patchDeployment applies a patch request to a k8s target deployment
func (p *patcher) patchDeployment(req Request) error {
	if !p.isLeader() {
		log.Debug("Not leader, skipping")
		return nil
	}
	deploy, err := p.k8sClient.AppsV1().Deployments(req.K8sTarget.Namespace).Get(context.TODO(), req.K8sTarget.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	revision := strconv.FormatInt(req.Revision, 10)
	if deploy.Annotations != nil && deploy.Annotations[k8sutil.RcIDAnnotKey] == req.ID && deploy.Annotations[k8sutil.RcRevisionAnnotKey] == revision {
		log.Infof("Remote Config ID %q with revision %q has already been applied to object %s, skipping", req.ID, revision, req.K8sTarget)
		return nil
	}
	log.Infof("Applying Remote Config ID %q with revision %q and action %q to object %s", req.ID, revision, req.Action, req.K8sTarget)

	target := workloadpatcher.DeploymentTarget(req.K8sTarget.Namespace, req.K8sTarget.Name)
	intent := workloadpatcher.NewPatchIntent(target)

	switch req.Action {
	case StageConfig:
		// Consume the config without triggering a rolling update.
		log.Debugf("Remote Config ID %q with revision %q has a \"stage\" action. The pod template won't be patched, only the deployment annotations", req.ID, revision)
	case EnableConfig:
		conf, err := json.Marshal(req.LibConfig)
		if err != nil {
			return fmt.Errorf("failed to encode library config: %v", err)
		}
		versionAnnotKey := annotation.LibraryVersion.Format(req.LibConfig.Language)
		configAnnotKey := annotation.LibraryConfigV1.Format(req.LibConfig.Language)

		intent = intent.
			With(workloadpatcher.SetPodTemplateLabels(map[string]interface{}{
				common.EnabledLabelKey: "true",
			})).
			With(workloadpatcher.SetPodTemplateAnnotations(map[string]interface{}{
				versionAnnotKey:            req.LibConfig.Version,
				configAnnotKey:             string(conf),
				k8sutil.RcIDAnnotKey:       req.ID,
				k8sutil.RcRevisionAnnotKey: revision,
			}))
	case DisableConfig:
		versionAnnotKey := annotation.LibraryVersion.Format(req.LibConfig.Language)
		configAnnotKey := annotation.LibraryConfigV1.Format(req.LibConfig.Language)

		intent = intent.
			With(workloadpatcher.SetPodTemplateLabels(map[string]interface{}{
				common.EnabledLabelKey: "false",
			})).
			With(workloadpatcher.DeletePodTemplateAnnotations([]string{
				versionAnnotKey,
				configAnnotKey,
			})).
			With(workloadpatcher.SetPodTemplateAnnotations(map[string]interface{}{
				k8sutil.RcIDAnnotKey:       req.ID,
				k8sutil.RcRevisionAnnotKey: revision,
			}))
	default:
		return fmt.Errorf("unknown action %q", req.Action)
	}

	// Always set deployment-level RC tracking annotations
	intent = intent.With(workloadpatcher.SetMetadataAnnotations(map[string]interface{}{
		k8sutil.RcIDAnnotKey:       req.ID,
		k8sutil.RcRevisionAnnotKey: revision,
	}))

	_, err = p.patchClient.Apply(context.TODO(), intent, workloadpatcher.PatchOptions{
		Caller:    "rc_patcher",
		PatchType: types.StrategicMergePatchType,
	})
	if err != nil {
		p.telemetryCollector.SendRemoteConfigMutateEvent(req.getApmRemoteConfigEvent(err, telemetry.FailedToMutateConfig))
		return err
	}
	p.telemetryCollector.SendRemoteConfigMutateEvent(req.getApmRemoteConfigEvent(nil, telemetry.Success))
	metrics.PatchCompleted.Inc()
	return nil
}
