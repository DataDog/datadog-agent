// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubernetes

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
)

type KubernetesObjRefOutput struct { // nolint:revive, We want to keep the name as <Component>ObjRefOutput
	components.JSONImporter

	Namespace      string            `json:"namespace"`
	Name           string            `json:"name"`
	Kind           string            `json:"kind"`
	AppVersion     string            `json:"installAppVersion"`
	Version        string            `json:"installVersion"`
	LabelSelectors map[string]string `json:"labelSelectors"`
}

type KubernetesObjectRef struct { // nolint:revive, We want to keep the name as <Component>ObjectRef
	pulumi.ResourceState
	components.Component

	Namespace      pulumi.String       `pulumi:"namespace"`
	Name           pulumi.String       `pulumi:"name"`
	Kind           pulumi.String       `pulumi:"kind"`
	AppVersion     pulumi.StringOutput `pulumi:"installAppVersion"`
	Version        pulumi.StringOutput `pulumi:"installVersion"`
	LabelSelectors pulumi.Map          `pulumi:"labelSelectors"`
}

func NewKubernetesObjRef(e config.Env, name string, namespace string, kind string, appVersion pulumi.StringOutput, version pulumi.StringOutput, labelSelectors map[string]string) (*KubernetesObjectRef, error) {
	return components.NewComponent(e, name, func(comp *KubernetesObjectRef) error {
		comp.Name = pulumi.String(name)
		comp.Namespace = pulumi.String(namespace)
		comp.Kind = pulumi.String(kind)
		comp.AppVersion = appVersion
		comp.Version = version

		labelSelectorsMap := make(pulumi.Map)
		for k, v := range labelSelectors {
			labelSelectorsMap[k] = pulumi.String(v)
		}
		comp.LabelSelectors = labelSelectorsMap

		return nil
	})
}

func (h *KubernetesObjectRef) Export(ctx *pulumi.Context, out *KubernetesObjRefOutput) error {
	return components.Export(ctx, h, out)
}
