// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	appsGroup        = "apps"
	argoRolloutGroup = "argoproj.io"
	kedaGroup        = "keda.sh"
	kedaScaledObject = "ScaledObject"
)

type autoscalerParser struct {
	kind          workloadmeta.AutoscalerKind
	targetRefPath []string
}

// NewAutoscalerParser returns a parser for autoscaler resources.
func NewAutoscalerParser(kind workloadmeta.AutoscalerKind) (ObjectParser, error) {
	switch kind {
	case workloadmeta.AutoscalerKindHPA, workloadmeta.AutoscalerKindWPA:
		return autoscalerParser{kind: kind, targetRefPath: []string{"spec", "scaleTargetRef"}}, nil
	case workloadmeta.AutoscalerKindVPA, workloadmeta.AutoscalerKindDPA:
		return autoscalerParser{kind: kind, targetRefPath: []string{"spec", "targetRef"}}, nil
	default:
		return nil, fmt.Errorf("unsupported autoscaler kind %q", kind)
	}
}

func (p autoscalerParser) Parse(obj interface{}) workloadmeta.Entity {
	u := obj.(*unstructured.Unstructured)

	kind := p.kind
	if kind == workloadmeta.AutoscalerKindHPA && isOwnedByKEDA(u) {
		kind = workloadmeta.AutoscalerKindKEDA
	}

	targetAPIVersion, _, _ := unstructured.NestedString(u.Object, append(p.targetRefPath, "apiVersion")...)
	targetKind, _, _ := unstructured.NestedString(u.Object, append(p.targetRefPath, "kind")...)
	targetName, _, _ := unstructured.NestedString(u.Object, append(p.targetRefPath, "name")...)

	autoscalerTargetKind := supportedAutoscalerTargetKind(targetAPIVersion, targetKind)
	if targetName == "" {
		autoscalerTargetKind = ""
	}

	return &workloadmeta.KubernetesAutoscaler{
		EntityID: workloadmeta.GenerateKubernetesAutoscalerEntityID(kind, u.GetNamespace(), u.GetName()),
		EntityMeta: workloadmeta.EntityMeta{
			// Only the fields needed to identify the autoscaler. Labels,
			// annotations are left out on purpose.
			Name:      u.GetName(),
			Namespace: u.GetNamespace(),
		},
		AutoscalerKind: kind,
		TargetKind:     autoscalerTargetKind,
		TargetName:     targetName,
	}
}

// isOwnedByKEDA returns whether the object is owned by a KEDA ScaledObject.
// KEDA creates an HPA for each ScaledObject.
func isOwnedByKEDA(u *unstructured.Unstructured) bool {
	for _, owner := range u.GetOwnerReferences() {
		gv, err := schema.ParseGroupVersion(owner.APIVersion)
		if err != nil {
			continue
		}
		if gv.Group == kedaGroup && owner.Kind == kedaScaledObject {
			return true
		}
	}
	return false
}

// supportedAutoscalerTargetKind returns the target kind for the supported
// workloads (Deployments, StatefulSets and Argo Rollouts), or an empty string
// otherwise.
func supportedAutoscalerTargetKind(apiVersion, kind string) workloadmeta.AutoscalerTargetKind {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return ""
	}

	// The apiVersion is optional in the VPA target ref. For simplicity, this
	// applies to all autoscaler kinds.
	isApps := gv.Group == appsGroup || apiVersion == ""

	switch {
	case kind == kubernetes.DeploymentKind && isApps:
		return workloadmeta.AutoscalerTargetDeployment
	case kind == kubernetes.StatefulSetKind && isApps:
		return workloadmeta.AutoscalerTargetStatefulSet
	case kind == kubernetes.RolloutKind && gv.Group == argoRolloutGroup:
		return workloadmeta.AutoscalerTargetArgoRollout
	default:
		return ""
	}
}
