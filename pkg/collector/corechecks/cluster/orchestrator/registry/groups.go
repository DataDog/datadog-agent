// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package registry

// API groups of the custom resources collected by default.
const (
	DatadogAPIGroup        = "datadoghq.com"
	ArgoAPIGroup           = "argoproj.io"
	FluxAPIGroup           = "source.toolkit.fluxcd.io"
	FluxKustomizeAPIGroup  = "kustomize.toolkit.fluxcd.io"
	KarpenterAPIGroup      = "karpenter.sh"
	KarpenterAWSAPIGroup   = "karpenter.k8s.aws"
	KarpenterAzureAPIGroup = "karpenter.azure.com"
	EKSAPIGroup            = "eks.amazonaws.com"
	DynamoAPIGroup         = "nvidia.com"
	KubeRayAPIGroup        = "ray.io"
	KubeAIAPIGroup         = "kubeai.org"
	KServeAPIGroup         = "serving.kserve.io"
	GAIEAPIGroup           = "inference.networking.k8s.io"

	// Gateway API
	GatewayAPIGroup = "gateway.networking.k8s.io"

	// Service mesh
	IstioNetworkingAPIGroup = "networking.istio.io"
	EnvoyGatewayAPIGroup    = "gateway.envoyproxy.io"
	TraefikLegacyAPIGroup   = "traefik.containo.us"
	LinkerdPolicyAPIGroup   = "policy.linkerd.io"
	ConsulAPIGroup          = "consul.hashicorp.com"
	ConsulMeshAPIGroup      = "mesh.consul.hashicorp.com"
	KumaAPIGroup            = "kuma.io"

	// Ingress controllers
	NginxAPIGroup            = "k8s.nginx.org"
	TraefikAPIGroup          = "traefik.io"
	KongAPIGroup             = "configuration.konghq.com"
	HAProxyCoreAPIGroup      = "core.haproxy.org"
	HAProxyIngressV1APIGroup = "ingress.v1.haproxy.org"
)
