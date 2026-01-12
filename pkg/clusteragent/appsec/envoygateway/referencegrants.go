// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package envoygateway

import (
	"context"
	"encoding/json"
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

const referenceGrantName = "datadog-appsec-extproc"

var referenceGrantGVR = schema.GroupVersionResource{Resource: "referencegrants", Group: "gateway.networking.k8s.io", Version: "v1beta1"}

// grantManager manages the lifecycle of a single ReferenceGrant resource that lives alongside the envoy extproc deployment service
type grantManager struct {
	client dynamic.Interface
	logger log.Component
	eventRecorder

	namespace         string
	serviceName       string
	commonLabels      map[string]string
	commonAnnotations map[string]string
}

// createGrant is called by AddNamespaceToGrant if the ReferenceGrant does not already exist
func (g *grantManager) createGrant(ctx context.Context, dstNamespace string) error {
	grant := gwapiv1beta1.ReferenceGrant{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.networking.k8s.io/v1beta1",
			Kind:       "ReferenceGrant",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        referenceGrantName,
			Namespace:   g.namespace,
			Labels:      g.commonLabels,
			Annotations: g.commonAnnotations,
		},
		Spec: gwapiv1beta1.ReferenceGrantSpec{
			From: []gwapiv1beta1.ReferenceGrantFrom{
				{
					Group:     "gateway.envoyproxy.io",
					Kind:      "EnvoyExtensionPolicy",
					Namespace: gwapiv1beta1.Namespace(dstNamespace),
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
		g.logger.Debug("Reference grant already exists")
		// If the ReferenceGrant already exists, we consider it a success and try to patch it, even it means doing a redundant operation
		return g.AddNamespaceToGrant(ctx, dstNamespace)
	}

	if err != nil {
		g.recordReferenceGrantCreateFailed(dstNamespace, dstNamespace, err)
		return err
	}

	g.recordReferenceGrantCreated(dstNamespace, dstNamespace)
	return nil
}

func (g *grantManager) AddNamespaceToGrant(ctx context.Context, namespace string) error {
	if namespace == g.namespace {
		// Same namespace, no need for a ReferenceGrant
		return nil
	}

	jsonPatch := []map[string]any{{
		"op":   "add",
		"path": "/spec/from",
		"value": gwapiv1beta1.ReferenceGrantFrom{
			Group:     "gateway.envoyproxy.io",
			Kind:      "EnvoyExtensionPolicy",
			Namespace: gwapiv1beta1.Namespace(namespace),
		},
	}}

	jsonPatchBytes, err := json.Marshal(jsonPatch)
	if err != nil {
		return err
	}

	// TODO: handle case where more than 64 namespaces are present as this is the limit of "from" entries in a ReferenceGrant
	_, err = g.client.Resource(referenceGrantGVR).
		Namespace(g.namespace).
		Patch(ctx, referenceGrantName, types.JSONPatchType, jsonPatchBytes, metav1.PatchOptions{})

	if errors.IsNotFound(err) {
		g.logger.Debug("ReferenceGrant not found, creating it")
		return g.createGrant(ctx, namespace)
	}

	if err != nil {
		g.recordNamespaceAddFailed(namespace, namespace, err)
		return err
	}

	g.logger.Debugf("Adding namespace %q to grant %q", namespace, referenceGrantName)
	g.recordNamespaceAddedToGrant(namespace, namespace)

	return nil
}

func (g *grantManager) RemoveNamespaceToGrant(ctx context.Context, namespace string) error {
	if namespace == g.namespace {
		// Same namespace, no need for a ReferenceGrant
		return nil
	}

	unstructuredGrant, err := g.client.Resource(referenceGrantGVR).
		Namespace(g.namespace).
		Get(ctx, referenceGrantName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	var grant gwapiv1beta1.ReferenceGrant
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredGrant.Object, &grant); err != nil {
		return err
	}

	// Check if the namespace is present
	index := slices.IndexFunc(grant.Spec.From, func(item gwapiv1beta1.ReferenceGrantFrom) bool {
		return item.Namespace == gwapiv1beta1.Namespace(namespace)
	})

	if index == -1 {
		return fmt.Errorf("cannot remove the namespace %q from ReferenceGrant %q because it not in there", namespace, referenceGrantName)
	}

	if len(grant.Spec.From) == 1 {
		g.logger.Debug("Deleting ReferenceGrant as this was the last gateway")
		// This was the last gateway, we can have no choice but to delete the ReferenceGrant because we can't patch it to have no "from" entry
		err := g.client.Resource(referenceGrantGVR).
			Namespace(g.namespace).
			Delete(ctx, referenceGrantName, metav1.DeleteOptions{})

		if err != nil {
			g.recordReferenceGrantDeleteFailed(namespace, err)
			return err
		}
		return nil
	}

	// We need to patch the ReferenceGrant to remove the namespace
	jsonPatch := []map[string]string{{
		"op":   "remove",
		"path": fmt.Sprintf("/spec/from/%d", index),
	}}

	jsonPatchBytes, err := json.Marshal(jsonPatch)
	if err != nil {
		return err
	}

	g.logger.Debug("Patching ReferenceGrant to remove namespace: ", string(jsonPatchBytes))

	_, err = g.client.Resource(referenceGrantGVR).
		Namespace(g.namespace).
		Patch(ctx, referenceGrantName, types.JSONPatchType, jsonPatchBytes, metav1.PatchOptions{})

	if err != nil {
		g.recordNamespaceRemovalFailed(namespace, namespace, err)
		return err
	}

	g.recordNamespaceRemovedFromGrant(namespace, namespace)
	return nil
}
