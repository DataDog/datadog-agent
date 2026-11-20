// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package envoygateway

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"slices"

	gwapiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/ptr"
)

// legacyReferenceGrantName is the single ReferenceGrant older versions shared between all Gateway
// namespaces. Its spec.from is capped at 16 entries, so it is no longer created: it is only drained
// as the namespaces it lists lose their last Gateway.
const legacyReferenceGrantName = "datadog-appsec-extproc"

var referenceGrantGVR = schema.GroupVersionResource{Resource: "referencegrants", Group: "gateway.networking.k8s.io", Version: "v1beta1"}

// referenceGrantName returns the name of the ReferenceGrant that lets EnvoyExtensionPolicies in
// the given namespace reference the processor Service.
func referenceGrantName(fromNamespace string) string {
	return legacyReferenceGrantName + "-" + fromNamespace
}

// grantManager manages one ReferenceGrant per Gateway namespace, all living alongside the envoy extproc deployment service
type grantManager struct {
	client dynamic.Interface
	logger log.Component
	eventRecorder

	namespace         string
	serviceName       string
	commonLabels      map[string]string
	commonAnnotations map[string]string
}

// AddNamespaceToGrant ensures EnvoyExtensionPolicies in the given namespace can reference the processor Service
func (g *grantManager) AddNamespaceToGrant(ctx context.Context, namespace string) error {
	if namespace == g.namespace {
		// Same namespace, no need for a ReferenceGrant
		return nil
	}

	grant := gwapiv1beta1.ReferenceGrant{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.networking.k8s.io/v1beta1",
			Kind:       "ReferenceGrant",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        referenceGrantName(namespace),
			Namespace:   g.namespace,
			Labels:      g.commonLabels,
			Annotations: g.commonAnnotations,
		},
		Spec: gwapiv1beta1.ReferenceGrantSpec{
			From: []gwapiv1beta1.ReferenceGrantFrom{
				{
					Group:     "gateway.envoyproxy.io",
					Kind:      "EnvoyExtensionPolicy",
					Namespace: gwapiv1beta1.Namespace(namespace),
				}},
			To: []gwapiv1beta1.ReferenceGrantTo{
				{
					Kind: "Service",
					Name: ptr.To(gwapiv1beta1.ObjectName(g.serviceName)),
				},
			},
		},
	}

	// Look the grant up first: on a retry after policy creation failed, a redundant create could be
	// rejected by quota admission before reaching AlreadyExists
	existing, err := g.client.Resource(referenceGrantGVR).
		Namespace(g.namespace).
		Get(ctx, grant.Name, metav1.GetOptions{})
	if err == nil {
		return g.checkExistingGrant(existing, namespace, grant.Spec)
	}

	if !errors.IsNotFound(err) {
		return err
	}

	unstructuredGrant, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&grant)
	if err != nil {
		return err
	}

	_, err = g.client.Resource(referenceGrantGVR).
		Namespace(g.namespace).
		Create(ctx, &unstructured.Unstructured{Object: unstructuredGrant}, metav1.CreateOptions{})

	if errors.IsAlreadyExists(err) {
		// Created concurrently since the lookup
		existing, err := g.client.Resource(referenceGrantGVR).
			Namespace(g.namespace).
			Get(ctx, grant.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		return g.checkExistingGrant(existing, namespace, grant.Spec)
	}

	if err != nil {
		g.recordReferenceGrantCreateFailed(namespace, err)
		return err
	}

	g.recordReferenceGrantCreated(namespace)
	return nil
}

// RemoveNamespaceToGrant revokes the access granted to the given namespace, including any entry
// left for it in the legacy shared ReferenceGrant
func (g *grantManager) RemoveNamespaceToGrant(ctx context.Context, namespace string) error {
	if namespace == g.namespace {
		// Same namespace, no need for a ReferenceGrant
		return nil
	}

	name := referenceGrantName(namespace)
	grant, err := g.client.Resource(referenceGrantGVR).
		Namespace(g.namespace).
		Get(ctx, name, metav1.GetOptions{})
	switch {
	case errors.IsNotFound(err):
		err = nil
	case err != nil:
	case !appsecconfig.IsManagedByDatadog(grant.GetLabels()):
		g.logger.Warnf("Not deleting ReferenceGrant %s/%s: it was not created by the Datadog Cluster Agent", g.namespace, name)
	default:
		err = g.client.Resource(referenceGrantGVR).
			Namespace(g.namespace).
			Delete(ctx, name, deletePreconditions(grant))
		if errors.IsNotFound(err) {
			err = nil
		}
	}

	if err != nil {
		g.recordReferenceGrantDeleteFailed(namespace, name, err)
	}

	return stdErrors.Join(err, g.removeFromLegacyGrant(ctx, namespace))
}

// checkExistingGrant returns an error unless the existing grant for the namespace authorizes the
// reference described by want, since a grant created by someone else may allow something else.
func (g *grantManager) checkExistingGrant(unstructuredGrant *unstructured.Unstructured, namespace string, want gwapiv1beta1.ReferenceGrantSpec) error {
	name := unstructuredGrant.GetName()
	if unstructuredGrant.GetDeletionTimestamp() != nil {
		// The policy would lose backend access once the grant is gone, so retry until it can be recreated
		return fmt.Errorf("ReferenceGrant %s/%s is being deleted", g.namespace, name)
	}

	var existing gwapiv1beta1.ReferenceGrant
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredGrant.Object, &existing); err != nil {
		return err
	}

	wantTo := want.To[0]
	allowsTo := slices.ContainsFunc(existing.Spec.To, func(to gwapiv1beta1.ReferenceGrantTo) bool {
		return to.Group == wantTo.Group && to.Kind == wantTo.Kind && (to.Name == nil || *to.Name == *wantTo.Name)
	})
	if !allowsTo || !slices.Contains(existing.Spec.From, want.From[0]) {
		err := fmt.Errorf("ReferenceGrant %s/%s already exists but does not allow EnvoyExtensionPolicies in namespace %s to reference Service %s", g.namespace, name, namespace, g.serviceName)
		g.recordReferenceGrantCreateFailed(namespace, err)
		return err
	}

	g.logger.Debugf("ReferenceGrant %q already exists", name)
	return nil
}

// deletePreconditions makes a delete fail with a retryable Conflict when obj was replaced or changed
// after it was read, so a resource edited or recreated by someone else in between is not removed.
// Blank values would reject every delete.
func deletePreconditions(obj *unstructured.Unstructured) metav1.DeleteOptions {
	var preconditions metav1.Preconditions
	if uid := obj.GetUID(); uid != "" {
		preconditions.UID = &uid
	}
	if resourceVersion := obj.GetResourceVersion(); resourceVersion != "" {
		preconditions.ResourceVersion = &resourceVersion
	}
	if preconditions.UID == nil && preconditions.ResourceVersion == nil {
		return metav1.DeleteOptions{}
	}
	return metav1.DeleteOptions{Preconditions: &preconditions}
}

// removeFromLegacyGrant removes the namespace from the legacy shared ReferenceGrant, deleting the
// grant when it was the last entry. It is a no-op when the grant or the entry does not exist.
func (g *grantManager) removeFromLegacyGrant(ctx context.Context, namespace string) error {
	unstructuredGrant, err := g.client.Resource(referenceGrantGVR).
		Namespace(g.namespace).
		Get(ctx, legacyReferenceGrantName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return err
	}

	if !appsecconfig.IsManagedByDatadog(unstructuredGrant.GetLabels()) {
		g.logger.Warnf("Not updating ReferenceGrant %s/%s: it was not created by the Datadog Cluster Agent", g.namespace, legacyReferenceGrantName)
		return nil
	}

	var grant gwapiv1beta1.ReferenceGrant
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredGrant.Object, &grant); err != nil {
		return err
	}

	index := slices.IndexFunc(grant.Spec.From, func(item gwapiv1beta1.ReferenceGrantFrom) bool {
		return item.Namespace == gwapiv1beta1.Namespace(namespace)
	})

	if index == -1 {
		return nil
	}

	if len(grant.Spec.From) == 1 {
		g.logger.Debug("Deleting legacy ReferenceGrant as this was its last namespace")
		// A ReferenceGrant cannot be patched to have no "from" entry
		err := g.client.Resource(referenceGrantGVR).
			Namespace(g.namespace).
			Delete(ctx, legacyReferenceGrantName, deletePreconditions(unstructuredGrant))
		if errors.IsNotFound(err) {
			return nil
		}

		if err != nil {
			g.recordReferenceGrantDeleteFailed(namespace, legacyReferenceGrantName, err)
			return err
		}
		return nil
	}

	// Replace spec.from as read, guarded by the read resourceVersion: a concurrent edit makes the
	// patch fail with a retryable Conflict instead of removing another namespace's entry
	mergePatch := map[string]any{
		"spec": map[string]any{"from": slices.Delete(grant.Spec.From, index, index+1)},
	}
	if resourceVersion := unstructuredGrant.GetResourceVersion(); resourceVersion != "" {
		// An empty resourceVersion is a value in a merge patch, which the API server rejects
		mergePatch["metadata"] = map[string]any{"resourceVersion": resourceVersion}
	}

	mergePatchBytes, err := json.Marshal(mergePatch)
	if err != nil {
		return err
	}

	g.logger.Debug("Patching legacy ReferenceGrant to remove namespace: ", string(mergePatchBytes))

	_, err = g.client.Resource(referenceGrantGVR).
		Namespace(g.namespace).
		Patch(ctx, legacyReferenceGrantName, types.MergePatchType, mergePatchBytes, metav1.PatchOptions{})

	if err != nil {
		g.recordNamespaceRemovalFailed(namespace, err)
		return err
	}

	g.recordNamespaceRemovedFromGrant(namespace)
	return nil
}
