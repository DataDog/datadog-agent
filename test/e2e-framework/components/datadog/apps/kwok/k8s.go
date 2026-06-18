// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kwok deploys KWOK (Kubernetes WithOut Kubelet) so a cluster can host a
// large number of simulated nodes without paying for real compute.
package kwok

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

// kwokRepo is the GitHub repository hosting KWOK releases.
const kwokRepo = "kubernetes-sigs/kwok"

// K8sAppDefinition deploys the KWOK controller and the fast staging configuration
// from the latest upstream release.
//
// The manifests are referenced through GitHub's "/releases/latest/download/"
// redirect (resolved server-side when the Kubernetes provider fetches them) rather
// than resolving the release tag via the GitHub API. This drops the API
// rate-limit/availability dependency that the release lookup added to every
// up/preview/destroy, and keeps the Pulumi File inputs static and deterministic so
// a new upstream release no longer mutates the URL and forces resource replacement.
func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "kwok", k8sComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(k8sComponent))

	const releaseBaseURL = "https://github.com/" + kwokRepo + "/releases/latest/download/"

	kwok, err := yaml.NewConfigFile(e.Ctx(), "kwok", &yaml.ConfigFileArgs{
		File:            releaseBaseURL + "kwok.yaml",
		Transformations: []yaml.Transformation{relaxExemptFlowSchema},
	}, opts...)
	if err != nil {
		return nil, err
	}

	if res := kwok.GetResource("apiextensions.k8s.io/v1/CustomResourceDefinition", "stages.kwok.x-k8s.io", ""); res != nil {
		opts = append(opts, utils.PulumiDependsOn(res))
	}

	if _, err := yaml.NewConfigFile(e.Ctx(), "kwok-stage", &yaml.ConfigFileArgs{
		File: releaseBaseURL + "stage-fast.yaml",
	}, opts...); err != nil {
		return nil, err
	}

	return k8sComponent, nil
}

// relaxExemptFlowSchema rewrites KWOK's kwok-controller APF FlowSchema so it no longer
// references the built-in "exempt" PriorityLevelConfiguration, which managed API servers
// (e.g. EKS) reject for user-created FlowSchemas. The controller then falls under the
// default API Priority and Fairness handling instead of being exempt from it.
func relaxExemptFlowSchema(state map[string]interface{}, _ ...pulumi.ResourceOption) {
	if state["kind"] != "FlowSchema" {
		return
	}
	spec, ok := state["spec"].(map[string]interface{})
	if !ok {
		return
	}
	plc, ok := spec["priorityLevelConfiguration"].(map[string]interface{})
	if !ok {
		return
	}
	if plc["name"] == "exempt" {
		plc["name"] = "global-default"
	}
}
