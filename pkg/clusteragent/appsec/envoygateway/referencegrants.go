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

	unstructuredGrant, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&grant)
	if err != nil {
		return err
	}

	_, err = g.client.Resource(referenceGrantGVR).
		Namespace(g.namespace).
		Create(ctx, &unstructured.Unstructured{Object: unstructuredGrant}, metav1.CreateOptions{})

	if errors.IsAlreadyExists(err) {
		g.logger.Debugf("ReferenceGrant %q already exists", referenceGrantName(namespace))
		return nil
	}

	if err != nil {
		g.recordReferenceGrantCreateFailed(namespace, namespace, err)
		return err
	}

	g.recordReferenceGrantCreated(namespace, namespace)
	return nil
}

// RemoveNamespaceToGrant revokes the access granted to the given namespace, including any entry
// left for it in the legacy shared ReferenceGrant
func (g *grantManager) RemoveNamespaceToGrant(ctx context.Context, namespace string) error {
	if namespace == g.namespace {
		// Same namespace, no need for a ReferenceGrant
		return nil
	}

	err := g.client.Resource(referenceGrantGVR).
		Namespace(g.namespace).
		Delete(ctx, referenceGrantName(namespace), metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		err = nil
	}

	if err != nil {
		g.recordReferenceGrantDeleteFailed(namespace, referenceGrantName(namespace), err)
	}

	return stdErrors.Join(err, g.removeFromLegacyGrant(ctx, namespace))
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
			Delete(ctx, legacyReferenceGrantName, metav1.DeleteOptions{})
		if errors.IsNotFound(err) {
			return nil
		}

		if err != nil {
			g.recordReferenceGrantDeleteFailed(namespace, legacyReferenceGrantName, err)
			return err
		}
		return nil
	}

	jsonPatch := []map[string]string{{
		"op":   "remove",
		"path": fmt.Sprintf("/spec/from/%d", index),
	}}

	jsonPatchBytes, err := json.Marshal(jsonPatch)
	if err != nil {
		return err
	}

	g.logger.Debug("Patching legacy ReferenceGrant to remove namespace: ", string(jsonPatchBytes))

	_, err = g.client.Resource(referenceGrantGVR).
		Namespace(g.namespace).
		Patch(ctx, legacyReferenceGrantName, types.JSONPatchType, jsonPatchBytes, metav1.PatchOptions{})

	if err != nil {
		g.recordNamespaceRemovalFailed(namespace, namespace, err)
		return err
	}

	g.recordNamespaceRemovedFromGrant(namespace, namespace)
	return nil
}
