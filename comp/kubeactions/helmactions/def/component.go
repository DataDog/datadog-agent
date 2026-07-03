// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package helmactions provides a component for executing Helm actions.
package helmactions

import (
	"errors"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
)

// team: container-integrations

// Component is the component type.
type Component interface {
	// OnRollback is called when Job successfully scheduled
	OnRollback(in *RollbackInputs, job *batchv1.Job)
}

// RollbackInputs describes a single `helm rollback` invocation.
type RollbackInputs struct {
	// Release is the name of the Helm release to roll back. Required.
	Release string
	// ReleaseNamespace is the namespace of the Helm release. Required.
	ReleaseNamespace string
	// Revision is the target revision number. A value of 0 means "previous
	// revision" (helm's default behaviour).
	Revision int
	// JobNamespace is the namespace where the K8s Job will be created. Required.
	JobNamespace string
	// JobServiceAccountName is the service account the Job pod runs as. Required:
	// it must have the RBAC permissions helm needs to act on the release
	// (typically: read/write secrets in the release namespace, plus permissions
	// on the resources the chart manages).
	JobServiceAccountName string
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

func (o RollbackInputs) Validate() error {
	switch {
	case o.Release == "":
		return errors.New("release is required")
	case o.ReleaseNamespace == "":
		return errors.New("release namespace is required")
	case o.JobNamespace == "":
		return errors.New("job namespace is required")
	case o.JobServiceAccountName == "":
		return errors.New("service account name is required")
	case o.Revision < 0:
		return fmt.Errorf("revision must be >= 0, got %d", o.Revision)
	}
	return nil
}
