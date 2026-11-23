// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package registry

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sProcessors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

// resourceRegistry is the resource registry: every resource type that the orchestrator check collects. The types,
// constructors and options of its entries are in registry.go.
var resourceRegistry = []Resource{
	// Built-in resources
	builtin(orchestrator.K8sClusterRoleBinding, "rbac.authorization.k8s.io", "v1", "clusterrolebindings", withHandlers(k8sProcessors.NewClusterRoleBindingHandlers)),
	builtin(orchestrator.K8sClusterRole, "rbac.authorization.k8s.io", "v1", "clusterroles", withHandlers(k8sProcessors.NewClusterRoleHandlers)),
	builtin(orchestrator.K8sConfigMap, "", "v1", "configmaps", withManifestHandlers[*metav1.PartialObjectMetadata](), withInformer(configMapMetadataInformer), withEnabled(Never)),
	builtin(orchestrator.K8sCRD, "apiextensions.k8s.io", "v1", "customresourcedefinitions", withHandlerFunc(crdHandlers), withInformer(crdInformer), manifestOnly()),
	builtin(orchestrator.K8sCronJob, "batch", "v1", "cronjobs", withHandlerFunc(cronJobHandlers), withExtraVersions("v1beta1")),
	builtin(orchestrator.K8sDaemonSet, "apps", "v1", "daemonsets", withHandlers(k8sProcessors.NewDaemonSetHandlers)),
	builtin(orchestrator.K8sDeployment, "apps", "v1", "deployments", withHandlers(k8sProcessors.NewDeploymentHandlers)),
	builtin(orchestrator.K8sEndpointSlice, "discovery.k8s.io", "v1", "endpointslices", withInformer(DynamicInformer)),
	builtin(orchestrator.K8sConfigMap, "", "v1", "helm-releases-configmaps", withManifestHandlers[*corev1.ConfigMap](), withInformer(helmReleaseConfigMapInformer), withEnabled(setting("orchestrator_explorer.helm_releases.configmaps.enabled"))),
	builtin(orchestrator.K8sHorizontalPodAutoscaler, "autoscaling", "v2", "horizontalpodautoscalers", withHandlers(k8sProcessors.NewHorizontalPodAutoscalerHandlers)),
	builtin(orchestrator.K8sIngress, "networking.k8s.io", "v1", "ingresses", withHandlers(k8sProcessors.NewIngressHandlers)),
	builtin(orchestrator.K8sJob, "batch", "v1", "jobs", withHandlers(k8sProcessors.NewJobHandlers)),
	builtin(orchestrator.K8sLimitRange, "", "v1", "limitranges", withHandlers(k8sProcessors.NewLimitRangeHandlers)),
	builtin(orchestrator.K8sNamespace, "", "v1", "namespaces", withHandlers(k8sProcessors.NewNamespaceHandlers)),
	builtin(orchestrator.K8sNetworkPolicy, "networking.k8s.io", "v1", "networkpolicies", withHandlers(k8sProcessors.NewNetworkPolicyHandlers)),
	builtin(orchestrator.K8sNode, "", "v1", "nodes", withHandlers(k8sProcessors.NewNodeHandlers)),
	builtin(orchestrator.K8sPersistentVolumeClaim, "", "v1", "persistentvolumeclaims", withHandlers(k8sProcessors.NewPersistentVolumeClaimHandlers)),
	builtin(orchestrator.K8sPersistentVolume, "", "v1", "persistentvolumes", withHandlers(k8sProcessors.NewPersistentVolumeHandlers)),
	builtin(orchestrator.K8sPodDisruptionBudget, "policy", "v1", "poddisruptionbudgets", withHandlers(k8sProcessors.NewPodDisruptionBudgetHandlers)),
	// The pods collector lists unassigned pods. The node agents collect assigned pods.
	builtin(orchestrator.K8sPod, "", "v1", "pods", withHandlerFunc(podHandlers), withInformer(unassignedPodInformer), withoutDeletions()),
	builtin(orchestrator.K8sReplicaSet, "apps", "v1", "replicasets", withHandlers(k8sProcessors.NewReplicaSetHandlers)),
	builtin(orchestrator.K8sRoleBinding, "rbac.authorization.k8s.io", "v1", "rolebindings", withHandlers(k8sProcessors.NewRoleBindingHandlers)),
	builtin(orchestrator.K8sRole, "rbac.authorization.k8s.io", "v1", "roles", withHandlers(k8sProcessors.NewRoleHandlers)),
	builtin(orchestrator.K8sSecret, "", "v1", "secrets", withManifestHandlers[*metav1.PartialObjectMetadata](), withInformer(secretMetadataInformer), withEnabled(Never)),
	builtin(orchestrator.K8sServiceAccount, "", "v1", "serviceaccounts", withHandlers(k8sProcessors.NewServiceAccountHandlers)),
	builtin(orchestrator.K8sService, "", "v1", "services", withHandlers(k8sProcessors.NewServiceHandlers)),
	builtin(orchestrator.K8sStatefulSet, "apps", "v1", "statefulsets", withHandlers(k8sProcessors.NewStatefulSetHandlers)),
	builtin(orchestrator.K8sStorageClass, "storage.k8s.io", "v1", "storageclasses", withHandlers(k8sProcessors.NewStorageClassHandlers)),
	builtin(orchestrator.K8sPod, "", "v1", "terminated-pods", withHandlerFunc(podHandlers), withInformer(podDeletionWatcher), withEnabled(setting("orchestrator_explorer.terminated_pods.enabled"))),
	builtin(orchestrator.K8sVerticalPodAutoscaler, "autoscaling.k8s.io", "v1", "verticalpodautoscalers", withHandlers(k8sProcessors.NewVerticalPodAutoscalerHandlers), withInformer(vpaInformer)),

	// Datadog resources
	customResource(DatadogAPIGroup, "datadogslos", ootbEnabled, "v1alpha1"),
	customResource(DatadogAPIGroup, "datadogdashboards", ootbEnabled, "v1alpha1"),
	customResource(DatadogAPIGroup, "datadogagentprofiles", ootbEnabled, "v1alpha1"),
	customResource(DatadogAPIGroup, "datadogmonitors", ootbEnabled, "v1alpha1"),
	customResource(DatadogAPIGroup, "datadogmetrics", ootbEnabled, "v1alpha1"),
	customResource(DatadogAPIGroup, "datadogpodautoscalers", ootbEnabled, "v1alpha2"),
	customResource(DatadogAPIGroup, "datadogpodautoscalerclusterprofiles", ootbEnabled, "v1alpha2"),
	customResource(DatadogAPIGroup, "datadogagents", ootbEnabled, "v2alpha1"),
	customResource(DatadogAPIGroup, "datadoginstrumentations", ootbEnabled, "v1alpha1"),

	// Argo resources
	customResource(ArgoAPIGroup, "rollouts", ootbEnabled, "v1alpha1"),
	customResource(ArgoAPIGroup, "applications", ootbEnabled, "v1alpha1"),
	customResource(ArgoAPIGroup, "applicationsets", ootbEnabled, "v1alpha1"),
	// appprojects also exists, but unclear if they are need for resource location identification.

	// Flux resources
	customResource(FluxAPIGroup, "buckets", ootbEnabled, "v1"),
	customResource(FluxAPIGroup, "helmcharts", ootbEnabled, "v1"),
	customResource(FluxAPIGroup, "externalartifacts", ootbEnabled, "v1"),
	customResource(FluxAPIGroup, "gitrepositories", ootbEnabled, "v1"),
	customResource(FluxAPIGroup, "helmrepositories", ootbEnabled, "v1"),
	customResource(FluxAPIGroup, "ocirepositories", ootbEnabled, "v1"),
	customResource(FluxKustomizeAPIGroup, "kustomizations", ootbEnabled, "v1"),

	// Karpenter resources (empty resource = all resources in group)
	customResource(KarpenterAPIGroup, "", ootbEnabled, "v1"),
	customResource(KarpenterAWSAPIGroup, "", ootbEnabled, "v1"),
	customResource(KarpenterAzureAPIGroup, "", ootbEnabled, "v1beta1"),

	// EKS Auto Mode resources (for now only nodeclasses, but we can easily add more in the future if needed)
	customResource(EKSAPIGroup, "nodeclasses", ootbEnabled, "v1", "v1beta1"),

	// Dynamo resources
	customResource(DynamoAPIGroup, "dynamocheckpoints", ootbEnabled, "v1alpha1"),
	customResource(DynamoAPIGroup, "dynamocomponentdeployments", ootbEnabled, "v1beta1", "v1alpha1"),
	customResource(DynamoAPIGroup, "dynamographdeploymentrequests", ootbEnabled, "v1beta1", "v1alpha1"),
	customResource(DynamoAPIGroup, "dynamographdeployments", ootbEnabled, "v1beta1", "v1alpha1"),
	customResource(DynamoAPIGroup, "dynamographdeploymentscalingadapters", ootbEnabled, "v1beta1", "v1alpha1"),
	customResource(DynamoAPIGroup, "dynamomodels", ootbEnabled, "v1alpha1"),
	customResource(DynamoAPIGroup, "dynamoworkermetadatas", ootbEnabled, "v1alpha1"),

	// KubeRay resources
	customResource(KubeRayAPIGroup, "rayclusters", ootbEnabled, "v1", "v1alpha1"),
	customResource(KubeRayAPIGroup, "raycronjobs", ootbEnabled, "v1"),
	customResource(KubeRayAPIGroup, "rayjobs", ootbEnabled, "v1", "v1alpha1"),
	customResource(KubeRayAPIGroup, "rayservices", ootbEnabled, "v1", "v1alpha1"),

	// KubeAI resources
	customResource(KubeAIAPIGroup, "models", ootbEnabled, "v1"),

	// KServe resources
	customResource(KServeAPIGroup, "clusterstoragecontainers", ootbEnabled, "v1alpha1"),
	customResource(KServeAPIGroup, "llminferenceserviceconfigs", ootbEnabled, "v1alpha2", "v1alpha1"),
	customResource(KServeAPIGroup, "llminferenceservices", ootbEnabled, "v1alpha2", "v1alpha1"),
	customResource(KServeAPIGroup, "localmodelcaches", ootbEnabled, "v1alpha1"),
	customResource(KServeAPIGroup, "localmodelnamespacecaches", ootbEnabled, "v1alpha1"),
	customResource(KServeAPIGroup, "localmodelnodegroups", ootbEnabled, "v1alpha1"),
	customResource(KServeAPIGroup, "localmodelnodes", ootbEnabled, "v1alpha1"),
	customResource(KServeAPIGroup, "clusterservingruntimes", ootbEnabled, "v1alpha1"),
	customResource(KServeAPIGroup, "inferencegraphs", ootbEnabled, "v1alpha1"),
	customResource(KServeAPIGroup, "inferenceservices", ootbEnabled, "v1beta1"),
	customResource(KServeAPIGroup, "servingruntimes", ootbEnabled, "v1alpha1"),
	customResource(KServeAPIGroup, "trainedmodels", ootbEnabled, "v1alpha1"),

	// Gateway API Inference Extension resources
	customResource(GAIEAPIGroup, "inferencepools", ootbEnabled, "v1"),

	// Gateway API resources
	customResource(GatewayAPIGroup, "gateways", gatewayAPIEnabled, "v1", "v1beta1"),
	customResource(GatewayAPIGroup, "httproutes", gatewayAPIEnabled, "v1", "v1beta1"),
	customResource(GatewayAPIGroup, "grpcroutes", gatewayAPIEnabled, "v1", "v1alpha2"),
	customResource(GatewayAPIGroup, "tlsroutes", gatewayAPIEnabled, "v1alpha2"),
	customResource(GatewayAPIGroup, "listenersets", gatewayAPIEnabled, "v1alpha1"),

	// Service mesh — Istio (resource-specific to avoid over-collection)
	customResource(IstioNetworkingAPIGroup, "virtualservices", serviceMeshEnabled, "v1", "v1beta1", "v1alpha3"),
	customResource(IstioNetworkingAPIGroup, "gateways", serviceMeshEnabled, "v1", "v1beta1", "v1alpha3"),
	customResource(IstioNetworkingAPIGroup, "destinationrules", serviceMeshEnabled, "v1", "v1beta1", "v1alpha3"),
	customResource(IstioNetworkingAPIGroup, "serviceentries", serviceMeshEnabled, "v1", "v1beta1", "v1alpha3"),
	customResource(IstioNetworkingAPIGroup, "sidecars", serviceMeshEnabled, "v1", "v1beta1", "v1alpha3"),

	// Service mesh — other vendors (group-level, empty resource = all resources in group)
	customResource(EnvoyGatewayAPIGroup, "", serviceMeshEnabled, "v1alpha1"),
	customResource(TraefikLegacyAPIGroup, "", serviceMeshEnabled, "v1alpha1"),
	customResource(LinkerdPolicyAPIGroup, "", serviceMeshEnabled, "v1beta3", "v1beta2", "v1beta1"),
	customResource(ConsulAPIGroup, "", serviceMeshEnabled, "v1alpha1"),
	customResource(ConsulMeshAPIGroup, "", serviceMeshEnabled, "v2beta1"),
	customResource(KumaAPIGroup, "", serviceMeshEnabled, "v1alpha1"),

	// Ingress controllers — NGINX (resource-specific)
	customResource(NginxAPIGroup, "virtualservers", ingressControllersEnabled, "v1"),
	customResource(NginxAPIGroup, "virtualserverroutes", ingressControllersEnabled, "v1"),

	// Ingress controllers — Traefik (resource-specific)
	customResource(TraefikAPIGroup, "ingressroutes", ingressControllersEnabled, "v1alpha1"),

	// Ingress controllers — other vendors (group-level, empty resource = all resources in group)
	customResource(KongAPIGroup, "", ingressControllersEnabled, "v1", "v1beta1"),
	customResource(HAProxyCoreAPIGroup, "", ingressControllersEnabled, "v1alpha2"),
	customResource(HAProxyIngressV1APIGroup, "", ingressControllersEnabled, "v1"),
}
