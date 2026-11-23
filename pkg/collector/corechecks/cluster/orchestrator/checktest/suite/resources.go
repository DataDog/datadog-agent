// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	orchestratorcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
)

// Resource is a resource type that the check collects. Each Env checks that Resources lists the collectors of the
// check.
type Resource struct {
	// Group is the API group.
	Group string
	// Versions are the API versions, preferred first.
	Versions []string
	// Resource is the resource name. For a custom resource, an empty name selects every resource of the group.
	Resource string
	// Kind is the resource kind. Discovery provides the kind of custom resources.
	Kind string
	// NodeType is the payload type. Custom resources use orchestrator.K8sCR.
	NodeType pkgorchestratormodel.NodeType
	// Enabled reports whether the resource is collected when the check configuration does not select collectors. Nil
	// means always.
	Enabled func() bool
	// IgnoreDeletions reports that the check does not collect the deleted resources from the informer.
	IgnoreDeletions bool
}

// IsCustomResource reports whether the resource is a custom resource, selected through API discovery.
func (r Resource) IsCustomResource() bool {
	return r.NodeType == orchestrator.K8sCR
}

// isEnabled reports whether the resource is collected when the check configuration does not select collectors.
func (r Resource) isEnabled() bool {
	return r.Enabled == nil || r.Enabled()
}

// sampleResource is the resource that the fixtures of a group-wide custom resource serve.
const sampleResource = "samples"

// fixtureResource returns the resource that the fixtures of the resource serve.
func (r Resource) fixtureResource() string {
	if r.IsCustomResource() && r.Resource == "" {
		return sampleResource
	}
	return r.Resource
}

// FixtureName returns the collector full name of the fixture of the resource in the version.
func (r Resource) FixtureName(version string) string {
	return strings.TrimPrefix(r.Group+"/"+version+"/"+r.fixtureResource(), "/")
}

// Resources returns the resource types that the check collects.
func Resources() []Resource {
	return resources
}

var resources = []Resource{
	// Built-in resources
	builtin(orchestrator.K8sClusterRoleBinding, "rbac.authorization.k8s.io", "v1", "clusterrolebindings"),
	builtin(orchestrator.K8sClusterRole, "rbac.authorization.k8s.io", "v1", "clusterroles"),
	builtin(orchestrator.K8sConfigMap, "", "v1", "configmaps", withEnabled(never)),
	builtin(orchestrator.K8sCRD, "apiextensions.k8s.io", "v1", "customresourcedefinitions"),
	builtin(orchestrator.K8sCronJob, "batch", "v1", "cronjobs", withExtraVersions("v1beta1")),
	builtin(orchestrator.K8sDaemonSet, "apps", "v1", "daemonsets"),
	builtin(orchestrator.K8sDeployment, "apps", "v1", "deployments"),
	builtin(orchestrator.K8sEndpointSlice, "discovery.k8s.io", "v1", "endpointslices"),
	builtin(orchestrator.K8sConfigMap, "", "v1", "helm-releases-configmaps", withEnabled(setting("orchestrator_explorer.helm_releases.configmaps.enabled"))),
	builtin(orchestrator.K8sHorizontalPodAutoscaler, "autoscaling", "v2", "horizontalpodautoscalers"),
	builtin(orchestrator.K8sIngress, "networking.k8s.io", "v1", "ingresses"),
	builtin(orchestrator.K8sJob, "batch", "v1", "jobs"),
	builtin(orchestrator.K8sLimitRange, "", "v1", "limitranges"),
	builtin(orchestrator.K8sNamespace, "", "v1", "namespaces"),
	builtin(orchestrator.K8sNetworkPolicy, "networking.k8s.io", "v1", "networkpolicies"),
	builtin(orchestrator.K8sNode, "", "v1", "nodes"),
	builtin(orchestrator.K8sPersistentVolumeClaim, "", "v1", "persistentvolumeclaims"),
	builtin(orchestrator.K8sPersistentVolume, "", "v1", "persistentvolumes"),
	builtin(orchestrator.K8sPodDisruptionBudget, "policy", "v1", "poddisruptionbudgets"),
	// The pods collector lists unassigned pods. The node agents collect assigned pods.
	builtin(orchestrator.K8sPod, "", "v1", "pods", withoutDeletions()),
	builtin(orchestrator.K8sReplicaSet, "apps", "v1", "replicasets"),
	builtin(orchestrator.K8sRoleBinding, "rbac.authorization.k8s.io", "v1", "rolebindings"),
	builtin(orchestrator.K8sRole, "rbac.authorization.k8s.io", "v1", "roles"),
	builtin(orchestrator.K8sSecret, "", "v1", "secrets", withEnabled(never)),
	builtin(orchestrator.K8sServiceAccount, "", "v1", "serviceaccounts"),
	builtin(orchestrator.K8sService, "", "v1", "services"),
	builtin(orchestrator.K8sStatefulSet, "apps", "v1", "statefulsets"),
	builtin(orchestrator.K8sStorageClass, "storage.k8s.io", "v1", "storageclasses"),
	builtin(orchestrator.K8sPod, "", "v1", "terminated-pods", withEnabled(setting("orchestrator_explorer.terminated_pods.enabled"))),
	builtin(orchestrator.K8sVerticalPodAutoscaler, "autoscaling.k8s.io", "v1", "verticalpodautoscalers"),

	// Datadog resources
	customResource(orchestratorcheck.DatadogAPIGroupForTest, "datadogslos", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.DatadogAPIGroupForTest, "datadogdashboards", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.DatadogAPIGroupForTest, "datadogagentprofiles", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.DatadogAPIGroupForTest, "datadogmonitors", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.DatadogAPIGroupForTest, "datadogmetrics", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.DatadogAPIGroupForTest, "datadogpodautoscalers", ootbEnabled, "v1alpha2"),
	customResource(orchestratorcheck.DatadogAPIGroupForTest, "datadogpodautoscalerclusterprofiles", ootbEnabled, "v1alpha2"),
	customResource(orchestratorcheck.DatadogAPIGroupForTest, "datadogagents", ootbEnabled, "v2alpha1"),
	customResource(orchestratorcheck.DatadogAPIGroupForTest, "datadoginstrumentations", ootbEnabled, "v1alpha1"),

	// Argo resources
	customResource(orchestratorcheck.ArgoAPIGroup, "rollouts", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.ArgoAPIGroup, "applications", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.ArgoAPIGroup, "applicationsets", ootbEnabled, "v1alpha1"),
	// appprojects also exists, but unclear if they are need for resource location identification.

	// Flux resources
	customResource(orchestratorcheck.FluxAPIGroup, "buckets", ootbEnabled, "v1"),
	customResource(orchestratorcheck.FluxAPIGroup, "helmcharts", ootbEnabled, "v1"),
	customResource(orchestratorcheck.FluxAPIGroup, "externalartifacts", ootbEnabled, "v1"),
	customResource(orchestratorcheck.FluxAPIGroup, "gitrepositories", ootbEnabled, "v1"),
	customResource(orchestratorcheck.FluxAPIGroup, "helmrepositories", ootbEnabled, "v1"),
	customResource(orchestratorcheck.FluxAPIGroup, "ocirepositories", ootbEnabled, "v1"),
	customResource(orchestratorcheck.FluxKustomizeAPIGroup, "kustomizations", ootbEnabled, "v1"),

	// Karpenter resources (empty resource = all resources in group)
	customResource(orchestratorcheck.KarpenterAPIGroup, "", ootbEnabled, "v1"),
	customResource(orchestratorcheck.KarpenterAWSAPIGroup, "", ootbEnabled, "v1"),
	customResource(orchestratorcheck.KarpenterAzureAPIGroup, "", ootbEnabled, "v1beta1"),

	// EKS Auto Mode resources (for now only nodeclasses, but we can easily add more in the future if needed)
	customResource(orchestratorcheck.EKSAPIGroup, "nodeclasses", ootbEnabled, "v1", "v1beta1"),

	// Dynamo resources
	customResource(orchestratorcheck.DynamoAPIGroup, "dynamocheckpoints", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.DynamoAPIGroup, "dynamocomponentdeployments", ootbEnabled, "v1beta1", "v1alpha1"),
	customResource(orchestratorcheck.DynamoAPIGroup, "dynamographdeploymentrequests", ootbEnabled, "v1beta1", "v1alpha1"),
	customResource(orchestratorcheck.DynamoAPIGroup, "dynamographdeployments", ootbEnabled, "v1beta1", "v1alpha1"),
	customResource(orchestratorcheck.DynamoAPIGroup, "dynamographdeploymentscalingadapters", ootbEnabled, "v1beta1", "v1alpha1"),
	customResource(orchestratorcheck.DynamoAPIGroup, "dynamomodels", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.DynamoAPIGroup, "dynamoworkermetadatas", ootbEnabled, "v1alpha1"),

	// KubeRay resources
	customResource(orchestratorcheck.KubeRayAPIGroup, "rayclusters", ootbEnabled, "v1", "v1alpha1"),
	customResource(orchestratorcheck.KubeRayAPIGroup, "raycronjobs", ootbEnabled, "v1"),
	customResource(orchestratorcheck.KubeRayAPIGroup, "rayjobs", ootbEnabled, "v1", "v1alpha1"),
	customResource(orchestratorcheck.KubeRayAPIGroup, "rayservices", ootbEnabled, "v1", "v1alpha1"),

	// KubeAI resources
	customResource(orchestratorcheck.KubeAIAPIGroup, "models", ootbEnabled, "v1"),

	// KServe resources
	customResource(orchestratorcheck.KServeAPIGroup, "clusterstoragecontainers", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.KServeAPIGroup, "llminferenceserviceconfigs", ootbEnabled, "v1alpha2", "v1alpha1"),
	customResource(orchestratorcheck.KServeAPIGroup, "llminferenceservices", ootbEnabled, "v1alpha2", "v1alpha1"),
	customResource(orchestratorcheck.KServeAPIGroup, "localmodelcaches", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.KServeAPIGroup, "localmodelnamespacecaches", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.KServeAPIGroup, "localmodelnodegroups", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.KServeAPIGroup, "localmodelnodes", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.KServeAPIGroup, "clusterservingruntimes", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.KServeAPIGroup, "inferencegraphs", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.KServeAPIGroup, "inferenceservices", ootbEnabled, "v1beta1"),
	customResource(orchestratorcheck.KServeAPIGroup, "servingruntimes", ootbEnabled, "v1alpha1"),
	customResource(orchestratorcheck.KServeAPIGroup, "trainedmodels", ootbEnabled, "v1alpha1"),

	// Gateway API Inference Extension resources
	customResource(orchestratorcheck.GAIEAPIGroup, "inferencepools", ootbEnabled, "v1"),

	// Gateway API resources
	customResource(orchestratorcheck.GatewayAPIGroup, "gateways", gatewayAPIEnabled, "v1", "v1beta1"),
	customResource(orchestratorcheck.GatewayAPIGroup, "httproutes", gatewayAPIEnabled, "v1", "v1beta1"),
	customResource(orchestratorcheck.GatewayAPIGroup, "grpcroutes", gatewayAPIEnabled, "v1", "v1alpha2"),
	customResource(orchestratorcheck.GatewayAPIGroup, "tlsroutes", gatewayAPIEnabled, "v1alpha2"),
	customResource(orchestratorcheck.GatewayAPIGroup, "listenersets", gatewayAPIEnabled, "v1alpha1"),

	// Service mesh — Istio (resource-specific to avoid over-collection)
	customResource(orchestratorcheck.IstioNetworkingAPIGroup, "virtualservices", serviceMeshEnabled, "v1", "v1beta1", "v1alpha3"),
	customResource(orchestratorcheck.IstioNetworkingAPIGroup, "gateways", serviceMeshEnabled, "v1", "v1beta1", "v1alpha3"),
	customResource(orchestratorcheck.IstioNetworkingAPIGroup, "destinationrules", serviceMeshEnabled, "v1", "v1beta1", "v1alpha3"),
	customResource(orchestratorcheck.IstioNetworkingAPIGroup, "serviceentries", serviceMeshEnabled, "v1", "v1beta1", "v1alpha3"),
	customResource(orchestratorcheck.IstioNetworkingAPIGroup, "sidecars", serviceMeshEnabled, "v1", "v1beta1", "v1alpha3"),

	// Service mesh — other vendors (group-level, empty resource = all resources in group)
	customResource(orchestratorcheck.EnvoyGatewayAPIGroup, "", serviceMeshEnabled, "v1alpha1"),
	customResource(orchestratorcheck.TraefikLegacyAPIGroup, "", serviceMeshEnabled, "v1alpha1"),
	customResource(orchestratorcheck.LinkerdPolicyAPIGroup, "", serviceMeshEnabled, "v1beta3", "v1beta2", "v1beta1"),
	customResource(orchestratorcheck.ConsulAPIGroup, "", serviceMeshEnabled, "v1alpha1"),
	customResource(orchestratorcheck.ConsulMeshAPIGroup, "", serviceMeshEnabled, "v2beta1"),
	customResource(orchestratorcheck.KumaAPIGroup, "", serviceMeshEnabled, "v1alpha1"),

	// Ingress controllers — NGINX (resource-specific)
	customResource(orchestratorcheck.NginxAPIGroup, "virtualservers", ingressControllersEnabled, "v1"),
	customResource(orchestratorcheck.NginxAPIGroup, "virtualserverroutes", ingressControllersEnabled, "v1"),

	// Ingress controllers — Traefik (resource-specific)
	customResource(orchestratorcheck.TraefikAPIGroup, "ingressroutes", ingressControllersEnabled, "v1alpha1"),

	// Ingress controllers — other vendors (group-level, empty resource = all resources in group)
	customResource(orchestratorcheck.KongAPIGroup, "", ingressControllersEnabled, "v1", "v1beta1"),
	customResource(orchestratorcheck.HAProxyCoreAPIGroup, "", ingressControllersEnabled, "v1alpha2"),
	customResource(orchestratorcheck.HAProxyIngressV1APIGroup, "", ingressControllersEnabled, "v1"),
}

// builtin returns a built-in resource. Its kind is the name of its node type.
func builtin(nodeType pkgorchestratormodel.NodeType, group, version, resource string, options ...resourceOption) Resource {
	r := Resource{Group: group, Versions: []string{version}, Resource: resource, Kind: nodeType.String(), NodeType: nodeType}
	for _, option := range options {
		option(&r)
	}
	return r
}

// customResource returns a built-in custom resource.
func customResource(group, resource string, enabled func() bool, versions ...string) Resource {
	return Resource{Group: group, Versions: versions, Resource: resource, NodeType: orchestrator.K8sCR, Enabled: enabled}
}

type resourceOption func(*Resource)

func withEnabled(enabled func() bool) resourceOption {
	return func(r *Resource) { r.Enabled = enabled }
}

// withExtraVersions adds older versions, each collected by its own collector.
func withExtraVersions(versions ...string) resourceOption {
	return func(r *Resource) { r.Versions = append(r.Versions, versions...) }
}

func withoutDeletions() resourceOption {
	return func(r *Resource) { r.IgnoreDeletions = true }
}

func setting(key string) func() bool {
	return func() bool { return pkgconfigsetup.Datadog().GetBool(key) }
}

// never is the default selection of the resources that are only collected when the check configuration selects them.
func never() bool { return false }

var ootbEnabled = setting("orchestrator_explorer.custom_resources.ootb.enabled")

func gatewayAPIEnabled() bool {
	return ootbEnabled() && pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.custom_resources.ootb.gateway_api")
}

func serviceMeshEnabled() bool {
	return ootbEnabled() && pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.custom_resources.ootb.service_mesh")
}

func ingressControllersEnabled() bool {
	return ootbEnabled() && pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.custom_resources.ootb.ingress_controllers")
}

// requireResourcesMatchCheck fails the test unless Resources lists exactly the collectors of the check, so that every
// test covers every resource. The cluster is not listed, as its manifest combines the nodes.
func requireResourcesMatchCheck(t *testing.T, env *Env) {
	t.Helper()
	collectorNames := map[string]bool{}
	for _, m := range orchestratorcheck.CollectorMetadataForTest(env.Config, env.Workloadmeta, env.Tagger) {
		if m.Name != "clusters" {
			collectorNames[m.FullName()] = true
		}
	}
	for _, name := range orchestratorcheck.BuiltinCustomResourcesForTest() {
		collectorNames[name] = true
	}
	var listed []string
	for _, resource := range Resources() {
		for _, version := range resource.Versions {
			listed = append(listed, strings.TrimPrefix(resource.Group+"/"+version+"/"+resource.Resource, "/"))
		}
	}
	require.ElementsMatch(t, slices.Collect(maps.Keys(collectorNames)), listed, "suite.Resources must list the collectors of the check")
	for name := range NotListed {
		listed := slices.ContainsFunc(Resources(), func(r Resource) bool { return r.FixtureName(r.Versions[0]) == name })
		require.True(t, listed, "NotListed names %s, which is not a resource of the suite", name)
	}
}
