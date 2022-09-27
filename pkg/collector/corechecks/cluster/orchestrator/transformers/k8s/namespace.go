// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator
// +build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	corev1 "k8s.io/api/core/v1"
)

// ExtractNamespace returns the protobuf model corresponding to a Kubernetes Namespace resource.
func ExtractNamespace(ns *corev1.Namespace) *model.Namespace {
	message := &model.Namespace{
		Metadata: extractMetadata(&ns.ObjectMeta),
		// from https://github.com/kubernetes/kubernetes/blob/1e12d92a5179dbfeb455c79dbf9120c8536e5f9c/pkg/printers/internalversion/printers.go#L1350
		Status: string(ns.Status.Phase),
	}

	finalizers := ns.Spec.Finalizers
	if len(finalizers) > 0 {
		msgFinalizers := make([]string, len(finalizers))
		for i, finalizer := range finalizers {
			msgFinalizers[i] = string(finalizer)
		}
		message.Finalizers = msgFinalizers
	}

	return message
}
