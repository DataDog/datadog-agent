// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helmactions

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

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

	defaultTTLSecondsAfterFinished int32 = 3600
)

// RollbackOptions describes a single `helm rollback` invocation.
type RollbackOptions struct {
	// Release is the name of the Helm release to roll back. Required.
	Release string
	// ReleaseNamespace is the namespace of the Helm release. Required.
	ReleaseNamespace string
	// Revision is the target revision number. A value of 0 means "previous
	// revision" (helm's default behaviour).
	Revision int
	// JobNamespace is the namespace where the K8s Job will be created. Required.
	JobNamespace string
	// ServiceAccountName is the service account the Job pod runs as. Required:
	// it must have the RBAC permissions helm needs to act on the release
	// (typically: read/write secrets in the release namespace, plus permissions
	// on the resources the chart manages).
	ServiceAccountName string
	// Image overrides the helm container image. Defaults to DefaultHelmImage.
	Image string
	// Driver selects the helm storage backend that holds the release state.
	// When non-empty it is set as HELM_DRIVER on the Job container. Helm's
	// default is "secret"; "configmap" and "sql" are the other in-tree drivers.
	// Leave empty to inherit helm's default.
	Driver string
	// BackoffLimit overrides the Job's spec.backoffLimit. When nil, defaults to
	// 0 — a failed rollback is surfaced as a failed Job rather than retried,
	// because retrying produces another helm revision instead of being a no-op.
	BackoffLimit *int32
	// TTLSecondsAfterFinished overrides the Job's spec.ttlSecondsAfterFinished.
	// When nil, defaults to 1h so finished Jobs are garbage-collected by the
	// TTL controller.
	TTLSecondsAfterFinished *int32
	// ExtraLabels are added to the Job and the Pod template, merged on top of
	// the labels this package sets by default.
	ExtraLabels map[string]string
}

func (o RollbackOptions) validate() error {
	switch {
	case o.Release == "":
		return errors.New("release is required")
	case o.ReleaseNamespace == "":
		return errors.New("release namespace is required")
	case o.JobNamespace == "":
		return errors.New("job namespace is required")
	case o.ServiceAccountName == "":
		return errors.New("service account name is required")
	case o.Revision < 0:
		return fmt.Errorf("revision must be >= 0, got %d", o.Revision)
	}
	return nil
}

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
func (e *RollbackExecutor) Run(ctx context.Context, opts RollbackOptions) (*batchv1.Job, error) {
	if err := opts.validate(); err != nil {
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

func buildRollbackJob(opts RollbackOptions) *batchv1.Job {
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

	labels := map[string]string{
		labelManagedBy: "datadog-cluster-agent",
		labelComponent: "helm-rollback",
		labelRelease:   opts.Release,
		labelNamespace: opts.ReleaseNamespace,
	}
	for k, v := range opts.ExtraLabels {
		labels[k] = v
	}

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
					ServiceAccountName: opts.ServiceAccountName,
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
