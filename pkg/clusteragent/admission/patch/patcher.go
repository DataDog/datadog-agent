// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	k8sutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	jsonpatch "github.com/evanphx/json-patch"
	corev1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type patcher struct {
	k8sClient        kubernetes.Interface
	isLeader         func() bool
	deploymentsQueue chan PatchRequest
}

func newPatcher(k8sClient kubernetes.Interface, isLeaderFunc func() bool, pp patchProvider) *patcher {
	return &patcher{
		k8sClient:        k8sClient,
		isLeader:         isLeaderFunc,
		deploymentsQueue: pp.subscribe(KindDeployment),
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
func (p *patcher) patchDeployment(req PatchRequest) error {
	if !p.isLeader() {
		log.Debug("Not leader, skipping")
		return nil
	}
	deploy, err := p.k8sClient.AppsV1().Deployments(req.K8sTarget.Namespace).Get(context.TODO(), req.K8sTarget.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	oldObj, err := json.Marshal(deploy)
	if err != nil {
		return fmt.Errorf("failed to encode object: %v", err)
	}
	revision := fmt.Sprint(req.Revision)
	if deploy.Annotations == nil {
		deploy.Annotations = make(map[string]string)
	}
	if deploy.Annotations[k8sutil.RcIDAnnotKey] == req.ID && deploy.Annotations[k8sutil.RcRevisionAnnotKey] == revision {
		log.Infof("Remote Config ID %q with revision %q has already been applied to object %s, skipping", req.ID, revision, req.K8sTarget)
		return nil
	}
	log.Infof("Applying Remote Config ID %q with revision %q and action %q to object %s", req.ID, revision, req.Action, req.K8sTarget)
	switch req.Action {
	case StageConfig:
		// Consume the config without triggering a rolling update.
		log.Debugf("Remote Config ID %q with revision %q has a \"stage\" action. The pod template won't be patched, only the deployment annotations", req.ID, revision)
	case EnableConfig:
		if err := enableConfig(deploy, req); err != nil {
			return err
		}
	case DisableConfig:
		disableConfig(deploy, req)
	default:
		return fmt.Errorf("unknown action %q", req.Action)
	}
	deploy.Annotations[k8sutil.RcIDAnnotKey] = req.ID
	deploy.Annotations[k8sutil.RcRevisionAnnotKey] = revision
	newObj, err := json.Marshal(deploy)
	if err != nil {
		return fmt.Errorf("failed to encode object: %v", err)
	}
	patch, err := jsonpatch.CreateMergePatch(oldObj, newObj)
	if err != nil {
		return fmt.Errorf("failed to build the JSON patch: %v", err)
	}
	log.Infof("Patching %s with patch %s", req.K8sTarget, string(patch))
	if _, err = p.k8sClient.AppsV1().Deployments(req.K8sTarget.Namespace).Patch(context.TODO(), req.K8sTarget.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return err
	}
	metrics.PatchCompleted.Inc()
	return nil
}

func enableConfig(deploy *corev1.Deployment, req PatchRequest) error {
	if deploy.Spec.Template.Labels == nil {
		deploy.Spec.Template.Labels = make(map[string]string)
	}
	deploy.Spec.Template.Labels[common.EnabledLabelKey] = "true"
	if deploy.Spec.Template.Annotations == nil {
		deploy.Spec.Template.Annotations = make(map[string]string)
	}
	versionAnnotKey := fmt.Sprintf(common.LibVersionAnnotKeyFormat, req.LibConfig.Language)
	deploy.Spec.Template.Annotations[versionAnnotKey] = req.LibConfig.Version
	conf, err := json.Marshal(req.LibConfig)
	if err != nil {
		return fmt.Errorf("failed to encode library config: %v", err)
	}
	configAnnotKey := fmt.Sprintf(common.LibConfigV1AnnotKeyFormat, req.LibConfig.Language)
	deploy.Spec.Template.Annotations[configAnnotKey] = string(conf)
	deploy.Spec.Template.Annotations[k8sutil.RcIDAnnotKey] = req.ID
	deploy.Spec.Template.Annotations[k8sutil.RcRevisionAnnotKey] = fmt.Sprint(req.Revision)
	return nil
}

func disableConfig(deploy *corev1.Deployment, req PatchRequest) {
	if deploy.Spec.Template.Labels == nil {
		deploy.Spec.Template.Labels = make(map[string]string)
	}
	if val, found := deploy.Spec.Template.Labels[common.EnabledLabelKey]; found {
		log.Debugf("Found pod label %q=%q in target %s. Setting it to false", common.EnabledLabelKey, val, req.K8sTarget)
	}
	deploy.Spec.Template.Labels[common.EnabledLabelKey] = "false"
	versionAnnotKey := fmt.Sprintf(common.LibVersionAnnotKeyFormat, req.LibConfig.Language)
	delete(deploy.Spec.Template.Annotations, versionAnnotKey)
	configAnnotKey := fmt.Sprintf(common.LibConfigV1AnnotKeyFormat, req.LibConfig.Language)
	delete(deploy.Spec.Template.Annotations, configAnnotKey)
	deploy.Spec.Template.Annotations[k8sutil.RcIDAnnotKey] = req.ID
	deploy.Spec.Template.Annotations[k8sutil.RcRevisionAnnotKey] = fmt.Sprint(req.Revision)
}
