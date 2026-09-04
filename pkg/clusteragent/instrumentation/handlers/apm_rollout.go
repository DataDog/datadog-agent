// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	workloadpatcher "github.com/DataDog/datadog-agent/pkg/clusteragent/patcher"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	apmRestartedAtAnnotation = "kubectl.kubernetes.io/restartedAt"
	apmConfigHashAnnotation  = "internal.apm.datadoghq.com/datadoginstrumentation-config-hash"
)

// APMRolloutState describes what happened when the APM handler requested a rollout.
type APMRolloutState string

const (
	// APMRolloutTriggered means the target Deployment pod template was patched.
	APMRolloutTriggered APMRolloutState = "triggered"
	// APMRolloutAlreadyCurrent means the target Deployment already has the requested APM config hash.
	APMRolloutAlreadyCurrent APMRolloutState = "already_current"
	// APMRolloutSkipped means rollout was intentionally skipped, usually because this DCA is not leader.
	APMRolloutSkipped APMRolloutState = "skipped"
)

// APMRolloutResult is returned after trying to trigger an APM rollout.
type APMRolloutResult struct {
	State APMRolloutState
}

// APMRolloutPatcher triggers Deployment rollouts for APM DatadogInstrumentation targets.
type APMRolloutPatcher interface {
	RolloutDeployment(ctx context.Context, namespace, name, configHash string) (APMRolloutResult, error)
}

type deploymentRolloutPatcher struct {
	client      dynamic.Interface
	patchClient *workloadpatcher.Patcher
	isLeader    func() bool
	now         func() time.Time
}

// NewAPMDeploymentRolloutPatcher returns an APM rollout patcher backed by the Kubernetes dynamic client.
func NewAPMDeploymentRolloutPatcher(client dynamic.Interface, isLeader func() bool) APMRolloutPatcher {
	return &deploymentRolloutPatcher{
		client:      client,
		patchClient: workloadpatcher.NewPatcher(client, nil),
		isLeader:    isLeader,
		now:         time.Now,
	}
}

func (p *deploymentRolloutPatcher) RolloutDeployment(ctx context.Context, namespace, name, configHash string) (APMRolloutResult, error) {
	if p.isLeader != nil && !p.isLeader() {
		return APMRolloutResult{State: APMRolloutSkipped}, nil
	}

	target := workloadpatcher.DeploymentTarget(namespace, name)
	deployment, err := p.client.Resource(target.GVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return APMRolloutResult{}, err
	}

	existingHash, _, err := unstructured.NestedString(deployment.Object, "spec", "template", "metadata", "annotations", apmConfigHashAnnotation)
	if err != nil {
		log.Debugf("Failed to read APM DatadogInstrumentation rollout annotation from deployment %s/%s: %v", namespace, name, err)
	}
	if existingHash == configHash {
		return APMRolloutResult{State: APMRolloutAlreadyCurrent}, nil
	}

	intent := workloadpatcher.NewPatchIntent(target).With(workloadpatcher.SetPodTemplateAnnotations(map[string]interface{}{
		apmRestartedAtAnnotation: p.now().Format(time.RFC3339),
		apmConfigHashAnnotation:  configHash,
	}))

	_, err = p.patchClient.Apply(ctx, intent, workloadpatcher.PatchOptions{
		Caller:          "datadoginstrumentation_apm",
		RetryOnConflict: true,
	})
	if err != nil {
		return APMRolloutResult{}, err
	}

	return APMRolloutResult{State: APMRolloutTriggered}, nil
}
