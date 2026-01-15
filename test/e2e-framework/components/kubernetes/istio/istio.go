// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package istio

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/helm"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	apiext "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	kubeHelm "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type HelmComponent struct {
	pulumi.ResourceState

	IstioBaseHelmReleaseStatus    kubeHelm.ReleaseStatusOutput
	IstiodHelmReleaseStatus       kubeHelm.ReleaseStatusOutput
	IstioIngressHelmReleaseStatus kubeHelm.ReleaseStatusOutput
}

func NewHelmInstallation(e config.Env, opts ...pulumi.ResourceOption) (*HelmComponent, error) {
	helmComponent := &HelmComponent{}
	if err := e.Ctx().RegisterComponentResource("dd:istio", "istio", helmComponent, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(helmComponent))
	optsRoot := opts

	// Create namespace if necessary
	ns, err := corev1.NewNamespace(e.Ctx(), "istio-system", &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String("istio-system"),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}
	opts = append(opts, utils.PulumiDependsOn(ns))

	values := buildIstioBaseHelmValues()

	istioBase, err := helm.NewInstallation(e, helm.InstallArgs{
		RepoURL:     "https://istio-release.storage.googleapis.com/charts",
		ChartName:   "base",
		InstallName: "istio-base",
		Namespace:   "istio-system",
		Values:      pulumi.Map(values),
	}, opts...)
	if err != nil {
		return nil, err
	}

	helmComponent.IstioBaseHelmReleaseStatus = istioBase.Status

	opts = append(opts, utils.PulumiDependsOn(istioBase))

	istiod, err := helm.NewInstallation(e, helm.InstallArgs{
		RepoURL:     "https://istio-release.storage.googleapis.com/charts",
		ChartName:   "istiod",
		InstallName: "istiod",
		Namespace:   "istio-system",
	}, opts...)
	if err != nil {
		return nil, err
	}
	helmComponent.IstiodHelmReleaseStatus = istiod.Status

	nsIngress, err := corev1.NewNamespace(e.Ctx(), "istio-ingress", &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String("istio-ingress"),
		},
	}, append(optsRoot, utils.PulumiDependsOn(istiod))...)
	if err != nil {
		return nil, err
	}

	istioIngress, err := helm.NewInstallation(e, helm.InstallArgs{
		RepoURL:     "https://istio-release.storage.googleapis.com/charts",
		ChartName:   "gateway",
		InstallName: "istio-ingress",
		Namespace:   "istio-ingress",
	}, append(optsRoot, utils.PulumiDependsOn(nsIngress))...)
	if err != nil {
		return nil, err
	}
	helmComponent.IstioIngressHelmReleaseStatus = istioIngress.Status

	// patch the default namespace to inject istio by default
	defaultNs, err := corev1.GetNamespace(e.Ctx(), "default", pulumi.ID("default"), nil, append(optsRoot, utils.PulumiDependsOn(istioIngress))...)
	if err != nil {
		return nil, err
	}
	_, err = corev1.NewNamespacePatch(e.Ctx(), "default", &corev1.NamespacePatchArgs{
		Metadata: metav1.ObjectMetaPatchArgs{
			Name: defaultNs.Metadata.Name(),
			Labels: pulumi.StringMap{
				"istio-injection": pulumi.String("enabled"),
			},
		},
	}, append(optsRoot, utils.PulumiDependsOn(defaultNs))...)
	if err != nil {
		return nil, err
	}

	resourceOutputs := pulumi.Map{
		"IstioBaseHelmReleaseStatus":    istioBase.Status,
		"IstiodHelmReleaseStatus":       istiod.Status,
		"IstioIngressHelmReleaseStatus": istioIngress.Status,
	}

	if err := e.Ctx().RegisterResourceOutputs(helmComponent, resourceOutputs); err != nil {
		return nil, err
	}

	return helmComponent, nil
}

func NewHttpbinServiceInstallation(e config.Env, opts ...pulumi.ResourceOption) (*corev1.Service, error) {
	// deploy httpbin on default namespace
	httpbinServiceAccount, err := corev1.NewServiceAccount(e.Ctx(), "httpbin", &corev1.ServiceAccountArgs{
		Metadata: metav1.ObjectMetaArgs{Name: pulumi.String("httpbin")},
	}, opts...)
	if err != nil {
		return nil, err
	}
	httpbinDeploy, err := appsv1.NewDeployment(e.Ctx(), "httpbin", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("httpbin"),
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app":     pulumi.String("httpbin"),
					"version": pulumi.String("v1"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app":     pulumi.String("httpbin"),
						"version": pulumi.String("v1"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: pulumi.String("httpbin"),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Image: pulumi.String("ghcr.io/datadog/apps-go-httpbin:" + apps.Version),
							Name:  pulumi.String("httpbin"),
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									ContainerPort: pulumi.Int(8080),
								},
							},
						},
					},
				},
			},
		},
	}, append(opts, utils.PulumiDependsOn(httpbinServiceAccount))...)
	if err != nil {
		return nil, err
	}

	httpbinService, err := corev1.NewService(e.Ctx(), "httpbin", &corev1.ServiceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String("httpbin"),
			Labels: pulumi.StringMap{
				"app":     pulumi.String("httpbin"),
				"service": pulumi.String("httpbin"),
			},
		},
		Spec: &corev1.ServiceSpecArgs{
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(8000),
					Protocol:   pulumi.String("TCP"),
					TargetPort: pulumi.Any(8080),
				},
			},
			Selector: pulumi.StringMap{
				"app": pulumi.String("httpbin"),
			},
		},
	}, append(opts, utils.PulumiDependsOn(httpbinDeploy))...)
	if err != nil {
		return nil, err
	}
	return httpbinService, nil
}

func NewHttpbinGatewayRoutesInstallation(e config.CommonEnvironment, opts ...pulumi.ResourceOption) error {
	// create the gateway
	httpbinGateway, err := apiext.NewCustomResource(e.Ctx(), "gateway", &apiext.CustomResourceArgs{
		ApiVersion: pulumi.String("networking.istio.io/v1alpha3"),
		Kind:       pulumi.String("Gateway"),
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("httpbin-gateway"),
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": pulumi.Map{
				"selector": pulumi.Map{
					"istio": pulumi.String("ingress"),
				},
				"servers": pulumi.MapArray{
					pulumi.Map{
						"port": pulumi.Map{
							"number":   pulumi.IntPtr(80),
							"name":     pulumi.String("http"),
							"protocol": pulumi.String("HTTP"),
						},
						"hosts": pulumi.StringArray{pulumi.String("httpbin.example.com")},
					},
				},
			},
		},
	}, opts...)
	if err != nil {
		return err
	}

	// configure routes
	virtualService, err := apiext.NewCustomResource(e.Ctx(), "virtualservice", &apiext.CustomResourceArgs{
		ApiVersion: pulumi.String("networking.istio.io/v1alpha3"),
		Kind:       pulumi.String("VirtualService"),
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("httpbin"),
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": pulumi.Map{
				"hosts":    pulumi.Array{pulumi.String("httpbin.example.com")},
				"gateways": pulumi.Array{pulumi.String("httpbin-gateway")},
				"http": pulumi.Array{
					pulumi.Map{
						"match": pulumi.MapArray{
							pulumi.Map{
								"uri": pulumi.Map{
									"prefix": pulumi.String("/status"),
								},
							},
							pulumi.Map{
								"uri": pulumi.Map{
									"prefix": pulumi.String("/delay"),
								},
							},
						},
						"route": pulumi.Array{
							pulumi.Map{
								"destination": pulumi.Map{
									"port": pulumi.Map{
										"number": pulumi.IntPtr(8000),
									},
									"host": pulumi.String("httpbin"),
								},
							},
						},
					},
				},
			},
		},
	}, append(opts, utils.PulumiDependsOn(httpbinGateway))...)
	if err != nil {
		return err
	}

	lbService, err := corev1.GetService(e.Ctx(), "istio-ingress", pulumi.ID("istio-ingress/istio-ingress"), nil,
		append(opts, utils.PulumiDependsOn(virtualService))...)
	if err != nil {
		return err
	}

	e.Ctx().Export("serviceExternalIP", lbService.Status.LoadBalancer().Ingress())

	return nil
}

/*
 */

type HelmValues pulumi.Map

func buildIstioBaseHelmValues() HelmValues {
	return HelmValues{
		"defaultRevision": pulumi.StringPtr("default"),
	}
}
