// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package helmactionsimpl implements the helmactions component interface.
package helmactionsimpl

import (
	"context"
	"fmt"
	"maps"
	"strconv"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DefaultHelmImage is the container image used when RollbackOptions.Image
	// is empty. It must contain a `helm` binary on PATH.
	DefaultHelmImage = "alpine/helm:latest"

	helmContainerName     = "helm"
	rollbackJobNamePrefix = "helm-rollback-"

	labelManagedBy = "app.kubernetes.io/managed-by"
	labelComponent = "app.kubernetes.io/component"
	labelRelease   = "helmactions.datadoghq.com/release"
	labelNamespace = "helmactions.datadoghq.com/release-namespace"

	// managedByValue / componentValue are the identity-label values every
	// rollback Job (and its Pods) must carry. The Job/Pod watcher's
	// jobWatchSelector is derived from them so the two can never drift.
	managedByValue = "datadog-cluster-agent"
	componentValue = "helm-rollback"

	defaultTTLSecondsAfterFinished int32 = 3600
)

// RollbackExecutor creates Kubernetes Jobs that run `helm rollback`.
type RollbackExecutor struct {
	clientset kubernetes.Interface
}

// NewRollbackExecutor returns a RollbackExecutor that talks to the API server
// via clientset.
func NewRollbackExecutor(clientset kubernetes.Interface) *RollbackExecutor {
	return &RollbackExecutor{clientset: clientset}
}

// Run validates opts and creates a Job that runs `helm rollback <release>
// [<revision>] --namespace <release-namespace>`. It returns the created Job;
// callers that need to observe completion should watch the Job or its Pods.
func (e *RollbackExecutor) Run(ctx context.Context, opts helmactions.RollbackInputs) (*batchv1.Job, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	job := buildRollbackJob(opts)
	created, err := e.clientset.BatchV1().Jobs(opts.JobNamespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create helm rollback job in %s: %w", opts.JobNamespace, err)
	}
	log.Infof("[HelmActions] Created rollback job %s/%s for release %s/%s (revision=%d)",
		created.Namespace, created.Name, opts.ReleaseNamespace, opts.Release, opts.Revision)
	return created, nil
}

func buildRollbackJob(opts helmactions.RollbackInputs) *batchv1.Job {
	image := opts.Image
	if image == "" {
		image = DefaultHelmImage
	}

	backoffLimit := int32(0)
	if opts.BackoffLimit != nil {
		backoffLimit = *opts.BackoffLimit
	}

	ttl := defaultTTLSecondsAfterFinished
	if opts.TTLSecondsAfterFinished != nil {
		ttl = *opts.TTLSecondsAfterFinished
	}

	args := []string{"rollback", opts.Release}
	if opts.Revision > 0 {
		args = append(args, strconv.Itoa(opts.Revision))
	}
	args = append(args, "--namespace", opts.ReleaseNamespace)

	// Merge ExtraLabels first, then stamp the identity labels last so callers
	// cannot accidentally (or deliberately) push a rollback Job out of the
	// watcher's selector by shadowing labelManagedBy / labelComponent.
	labels := map[string]string{}
	maps.Copy(labels, opts.ExtraLabels)

	labels[labelManagedBy] = managedByValue
	labels[labelComponent] = componentValue
	labels[labelRelease] = opts.Release
	labels[labelNamespace] = opts.ReleaseNamespace

	var env []corev1.EnvVar
	if opts.Driver != "" {
		env = append(env, corev1.EnvVar{Name: "HELM_DRIVER", Value: opts.Driver})
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: rollbackJobNamePrefix,
			Namespace:    opts.JobNamespace,
			Labels:       labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: opts.JobServiceAccountName,
					Containers: []corev1.Container{
						{
							Name:    helmContainerName,
							Image:   image,
							Command: []string{"helm"},
							Args:    args,
							Env:     env,
						},
					},
				},
			},
		},
	}
}
