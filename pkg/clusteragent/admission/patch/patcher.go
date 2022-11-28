// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package patch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	jsonpatch "github.com/evanphx/json-patch"
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
			if err := p.patchDeployment(req); err != nil {
				log.Error(err.Error())
			}
		case <-stopCh:
			log.Info("Shutting down patcher")
			return
		}
	}
}

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
	versionAnnotKey := fmt.Sprintf(common.LibVersionAnnotKeyFormat, req.LibID.Language)
	configAnnotKey := fmt.Sprintf(common.LibConfigV1AnnotKeyFormat, req.LibID.Language)
	switch req.Action {
	case ApplyConfig:
		if deploy.Spec.Template.Labels == nil {
			deploy.Spec.Template.Labels = make(map[string]string)
		}
		deploy.Spec.Template.Labels[common.EnabledLabelKey] = "true"
		if deploy.Spec.Template.Annotations == nil {
			deploy.Spec.Template.Annotations = make(map[string]string)
		}
		deploy.Spec.Template.Annotations[versionAnnotKey] = req.LibID.Version
		conf, err := json.Marshal(req.LibConfig)
		if err != nil {
			return fmt.Errorf("failed to encode library config: %v", err)
		}
		deploy.Spec.Template.Annotations[configAnnotKey] = string(conf)
	case DisableInjection:
		if deploy.Spec.Template.Labels == nil {
			deploy.Spec.Template.Labels = make(map[string]string)
		}
		deploy.Spec.Template.Labels[common.EnabledLabelKey] = "false"
		delete(deploy.Spec.Template.Annotations, versionAnnotKey)
		delete(deploy.Spec.Template.Annotations, configAnnotKey)
	default:
		return fmt.Errorf("unknown action %q", req.Action)
	}
	newObj, err := json.Marshal(deploy)
	if err != nil {
		return fmt.Errorf("failed to encode object: %v", err)
	}
	patch, err := jsonpatch.CreateMergePatch(oldObj, newObj)
	if err != nil {
		return fmt.Errorf("failed to build the JSON patch: %v", err)
	}
	log.Infof("Patching %s with patch %s", req.K8sTarget, string(patch))
	_, err = p.k8sClient.AppsV1().Deployments(req.K8sTarget.Namespace).Patch(context.TODO(), req.K8sTarget.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	return err
}
