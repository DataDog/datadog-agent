// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"regexp"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// deniedUserPattern extracts the requesting identity from a Kubernetes API
// server RBAC denial message, e.g. `User "system:serviceaccount:datadog:
// datadog-cluster-agent" cannot create resource "secrets" ...`. The API
// server embeds this in the error message; there is no structured field for
// it in metav1.Status.
var deniedUserPattern = regexp.MustCompile(`[Uu]ser\s+"([^"]+)"`)

// RBACError wraps a Kubernetes API "forbidden" error with the verb and
// resource the caller was denied, so that callers can report exactly which
// RBAC permission is missing instead of guessing.
type RBACError struct {
	// Verb is the Kubernetes API verb that was denied, e.g. "create" or "update".
	Verb string
	// Resource is the plural resource name, e.g. "secrets" or "mutatingwebhookconfigurations".
	Resource string
	// Namespace is the namespace of the request, empty for cluster-scoped resources.
	Namespace string
	// Name is the name of the object the request targeted.
	Name string
	// Username is the identity the API server denied, e.g.
	// "system:serviceaccount:datadog:datadog-cluster-agent", extracted from
	// the API server's own denial message. Empty if the message didn't
	// contain a recognizable identity (e.g. an older or non-standard
	// API server response).
	Username string

	*apierrors.StatusError
}

// Error implements the error interface.
func (e *RBACError) Error() string {
	target := fmt.Sprintf("%s %q", e.Resource, e.Name)
	if e.Namespace != "" {
		target = fmt.Sprintf("%s in namespace %q", target, e.Namespace)
	}
	who := "the caller"
	if e.Username != "" {
		who = fmt.Sprintf("%q", e.Username)
	}
	return fmt.Sprintf("%s is missing %q permission on %s: %s", who, e.Verb, target, e.StatusError.Error())
}

// Unwrap allows errors.Is/errors.As to reach the underlying StatusError.
func (e *RBACError) Unwrap() error {
	return e.StatusError
}

// WrapIfForbidden wraps err with verb/resource/namespace/name context when
// err is a Kubernetes "forbidden" error, so the RBAC permission that is
// missing can be reported precisely. It returns err unchanged otherwise.
func WrapIfForbidden(err error, verb, resource, namespace, name string) error {
	if err == nil {
		return nil
	}
	statusErr, ok := err.(*apierrors.StatusError)
	if !ok || !apierrors.IsForbidden(statusErr) {
		return err
	}
	var username string
	if m := deniedUserPattern.FindStringSubmatch(statusErr.Status().Message); len(m) == 2 {
		username = m[1]
	}
	return &RBACError{
		Verb:        verb,
		Resource:    resource,
		Namespace:   namespace,
		Name:        name,
		Username:    username,
		StatusError: statusErr,
	}
}
