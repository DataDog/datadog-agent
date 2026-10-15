// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package istio

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	apiext "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/helm"
)

const (
	// AmbientDefaultVersion is the Istio chart version installed by NewAmbientHelmInstallation
	AmbientDefaultVersion = "1.29.8"
	gatewayAPIVersion     = "v1.3.0"
	istioChartsRepo       = "https://istio-release.storage.googleapis.com/charts"
)

// AmbientHelmComponent is an Istio ambient mode installation: base, istiod, istio-cni, and ztunnel
type AmbientHelmComponent struct {
	pulumi.ResourceState

	// Ztunnel is the last release installed; depend on it before enrolling workloads
	Ztunnel pulumi.Resource
}

// NewAmbientHelmInstallation installs the Gateway API CRDs and Istio in ambient mode.
// Namespaces join the mesh with the label istio.io/dataplane-mode=ambient.
func NewAmbientHelmInstallation(e config.Env, version string, opts ...pulumi.ResourceOption) (*AmbientHelmComponent, error) {
	if version == "" {
		version = AmbientDefaultVersion
	}
	component := &AmbientHelmComponent{}
	if err := e.Ctx().RegisterComponentResource("dd:istio", "istio-ambient", component, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(component))

	// waypoints are Gateway API resources
	gatewayCRDs, err := yaml.NewConfigFile(e.Ctx(), "gateway-api-crds", &yaml.ConfigFileArgs{
		File: "https://github.com/kubernetes-sigs/gateway-api/releases/download/" + gatewayAPIVersion + "/standard-install.yaml",
	}, opts...)
	if err != nil {
		return nil, err
	}

	ns, err := corev1.NewNamespace(e.Ctx(), "istio-system", &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{Name: pulumi.String("istio-system")},
	}, opts...)
	if err != nil {
		return nil, err
	}

	// the cni and ztunnel charts tolerate every taint by default, which schedules them on
	// EKS Fargate nodes, where DaemonSet pods stay Pending
	noTolerations := pulumi.Array{}
	releases := []struct {
		chart, name string
		values      pulumi.Map
	}{
		{"base", "istio-base", pulumi.Map{"defaultRevision": pulumi.String("default")}},
		// istiod requests 2Gi by default, more than the framework's default node can spare
		{"istiod", "istiod", pulumi.Map{
			"profile": pulumi.String("ambient"),
			"resources": pulumi.Map{"requests": pulumi.Map{
				"cpu":    pulumi.String("100m"),
				"memory": pulumi.String("256Mi"),
			}},
		}},
		{"cni", "istio-cni", pulumi.Map{"profile": pulumi.String("ambient"), "tolerations": noTolerations}},
		{"ztunnel", "ztunnel", pulumi.Map{"tolerations": noTolerations}},
	}
	var previous pulumi.Resource = ns
	for _, r := range releases {
		release, err := helm.NewInstallation(e, helm.InstallArgs{
			RepoURL:     istioChartsRepo,
			ChartName:   r.chart,
			InstallName: r.name,
			Namespace:   "istio-system",
			Values:      r.values,
			Version:     pulumi.StringPtr(version),
		}, append(opts, utils.PulumiDependsOn(previous, gatewayCRDs))...)
		if err != nil {
			return nil, err
		}
		previous = release
	}
	component.Ztunnel = previous

	if err := e.Ctx().RegisterResourceOutputs(component, pulumi.Map{}); err != nil {
		return nil, err
	}
	return component, nil
}

// NewWaypoint creates a waypoint for services in namespace, the equivalent of
// `istioctl waypoint apply -n <namespace> --enroll-namespace`. The namespace must
// carry the label istio.io/use-waypoint=waypoint.
func NewWaypoint(e config.Env, namespace string, opts ...pulumi.ResourceOption) (*apiext.CustomResource, error) {
	return apiext.NewCustomResource(e.Ctx(), "waypoint-"+namespace, &apiext.CustomResourceArgs{
		ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
		Kind:       pulumi.String("Gateway"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("waypoint"),
			Namespace: pulumi.String(namespace),
			Labels:    pulumi.StringMap{"istio.io/waypoint-for": pulumi.String("service")},
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": pulumi.Map{
				"gatewayClassName": pulumi.String("istio-waypoint"),
				"listeners": pulumi.Array{
					pulumi.Map{
						"name":     pulumi.String("mesh"),
						"port":     pulumi.Int(15008),
						"protocol": pulumi.String("HBONE"),
					},
				},
			},
		},
	}, opts...)
}
