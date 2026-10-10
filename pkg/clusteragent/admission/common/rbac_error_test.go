// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestWrapIfForbidden_Nil(t *testing.T) {
	assert.Nil(t, WrapIfForbidden(nil, "create", "secrets", "ns", "name"))
}

func TestWrapIfForbidden_NonForbiddenErrorIsUnchanged(t *testing.T) {
	err := errors.New("connection refused")

	got := WrapIfForbidden(err, "create", "secrets", "ns", "name")

	assert.Same(t, err, got)
}

func TestWrapIfForbidden_NonStatusErrorIsUnchanged(t *testing.T) {
	// Some other package's error type that happens to say "forbidden" in its
	// message shouldn't be mistaken for a real API server denial.
	err := errors.New("forbidden")

	got := WrapIfForbidden(err, "create", "secrets", "ns", "name")

	assert.Same(t, err, got)
}

func TestWrapIfForbidden_ForbiddenErrorIsWrapped(t *testing.T) {
	statusErr := apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "datadog-webhook-certificate", errors.New("denied"))

	got := WrapIfForbidden(statusErr, "create", "secrets", "datadog", "datadog-webhook-certificate")

	var rbacErr *RBACError
	require.ErrorAs(t, got, &rbacErr)
	assert.Equal(t, "create", rbacErr.Verb)
	assert.Equal(t, "secrets", rbacErr.Resource)
	assert.Equal(t, "datadog", rbacErr.Namespace)
	assert.Equal(t, "datadog-webhook-certificate", rbacErr.Name)
	assert.Empty(t, rbacErr.Username, "the underlying error here has no recognizable identity to extract")

	// Method promotion from the embedded *apierrors.StatusError means the
	// wrapped error is still recognized as Forbidden without unwrapping.
	assert.True(t, apierrors.IsForbidden(got))
}

func TestWrapIfForbidden_ExtractsUsernameFromAPIServerMessage(t *testing.T) {
	// This is the shape of a real RBAC denial from the Kubernetes API server:
	// the identity that was denied is embedded in the message text, since
	// metav1.Status has no structured field for it.
	underlying := errors.New(`User "system:serviceaccount:datadog:datadog-cluster-agent" cannot create resource "secrets" in API group "" in the namespace "datadog"`)
	statusErr := apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "datadog-webhook-certificate", underlying)

	got := WrapIfForbidden(statusErr, "create", "secrets", "datadog", "datadog-webhook-certificate")

	var rbacErr *RBACError
	require.ErrorAs(t, got, &rbacErr)
	assert.Equal(t, "system:serviceaccount:datadog:datadog-cluster-agent", rbacErr.Username)
}

func TestRBACError_Error(t *testing.T) {
	statusErr := apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "datadog-webhook-certificate", errors.New("denied"))

	err := WrapIfForbidden(statusErr, "create", "secrets", "datadog", "datadog-webhook-certificate")

	assert.Contains(t, err.Error(), `"create"`)
	assert.Contains(t, err.Error(), "secrets")
	assert.Contains(t, err.Error(), "datadog-webhook-certificate")
	assert.Contains(t, err.Error(), "datadog")
}
