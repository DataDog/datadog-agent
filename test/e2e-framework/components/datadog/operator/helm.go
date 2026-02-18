// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package operator

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/helm"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	kubeHelm "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v3"
)

// HelmInstallationArgs is the set of arguments for creating a new HelmInstallation component
type HelmInstallationArgs struct {
	// KubeProvider is the Kubernetes provider to use
	KubeProvider *kubernetes.Provider
	// Namespace is the namespace in which to install the operator
	Namespace string
	// ValuesYAML is used to provide installation-specific values
	ValuesYAML pulumi.AssetOrArchiveArray
	// OperatorFullImagePath is used to specify the full image path for the agent
	OperatorFullImagePath string
	// ChartPath is the chart name or local chart path.
	ChartPath string
	// RepoURL is the Helm repository URL to use for the remote operator installation.
	RepoURL string
}

type HelmComponent struct {
	pulumi.ResourceState

	LinuxHelmReleaseName   pulumi.StringPtrOutput
	LinuxHelmReleaseStatus kubeHelm.ReleaseStatusOutput
}

func NewHelmInstallation(e config.Env, args HelmInstallationArgs, opts ...pulumi.ResourceOption) (*HelmComponent, error) {
	apiKey := e.AgentAPIKey()
	appKey := e.AgentAPPKey()

	opts = append(opts, pulumi.Providers(args.KubeProvider), e.WithProviders(config.ProviderRandom), pulumi.DeletedWith(args.KubeProvider))

	helmComponent := &HelmComponent{}
	if err := e.Ctx().RegisterComponentResource("dd:operator", "operator", helmComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(helmComponent))

	// Create namespace if necessary
	ns, err := corev1.NewNamespace(e.Ctx(), args.Namespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(args.Namespace),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}
	opts = append(opts, utils.PulumiDependsOn(ns))

	// Create secret if necessary
	secret, err := corev1.NewSecret(e.Ctx(), "datadog-credentials", &corev1.SecretArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: ns.Metadata.Name(),
			Name:      pulumi.String("dda-datadog-credentials"),
		},
		StringData: pulumi.StringMap{
			"api-key": apiKey,
			"app-key": appKey,
		},
	}, opts...)
	if err != nil {
		return nil, err
	}
	opts = append(opts, utils.PulumiDependsOn(secret))

	// Create image pull secret if necessary
	var imgPullSecret *corev1.Secret
	if e.ImagePullRegistry() != "" {
		imgPullSecret, err = utils.NewImagePullSecret(e, args.Namespace, opts...)
		if err != nil {
			return nil, err
		}
		opts = append(opts, utils.PulumiDependsOn(imgPullSecret))
	}

	// Compute some values
	operatorImagePath := dockerOperatorFullImagePath(e, "", "")

	if args.OperatorFullImagePath != "" {
		operatorImagePath = args.OperatorFullImagePath
	}
	operatorImagePath, operatorImageTag := utils.ParseImageReference(operatorImagePath)
	linuxInstallName := "datadog-operator-linux"

	values := buildLinuxHelmValues(operatorImagePath, operatorImageTag)
	values.configureImagePullSecret(imgPullSecret)

	defaultYAMLValues := values.toYAMLPulumiAssetOutput()

	var valuesYAML pulumi.AssetOrArchiveArray
	valuesYAML = append(valuesYAML, defaultYAMLValues)
	valuesYAML = append(valuesYAML, args.ValuesYAML...)

	linux, err := helm.NewInstallation(e, helm.InstallArgs{
		RepoURL:     args.RepoURL,
		ChartName:   args.ChartPath,
		InstallName: linuxInstallName,
		Namespace:   args.Namespace,
		ValuesYAML:  valuesYAML,
	}, opts...)
	if err != nil {
		return nil, err
	}

	helmComponent.LinuxHelmReleaseName = linux.Name
	helmComponent.LinuxHelmReleaseStatus = linux.Status

	resourceOutputs := pulumi.Map{
		"linuxHelmReleaseName":   linux.Name,
		"linuxHelmReleaseStatus": linux.Status,
	}

	if err := e.Ctx().RegisterResourceOutputs(helmComponent, resourceOutputs); err != nil {
		return nil, err
	}

	return helmComponent, nil
}

type HelmValues pulumi.Map

func buildLinuxHelmValues(operatorImagePath string, operatorImageTag string) HelmValues {
	return HelmValues{
		"apiKeyExistingSecret": pulumi.String("dda-datadog-credentials"),
		"appKeyExistingSecret": pulumi.String("dda-datadog-credentials"),
		"image": pulumi.Map{
			"repository":    pulumi.String(operatorImagePath),
			"tag":           pulumi.String(operatorImageTag),
			"doNotCheckTag": pulumi.Bool(true),
		},
		"logLevel": pulumi.String("debug"),
		"introspection": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"datadogAgentProfile": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"supportExtendedDaemonset": pulumi.Bool(false),
		"operatorMetricsEnabled":   pulumi.Bool(true),
		"metricsPort":              pulumi.Int(8383),
		"datadogAgent": pulumi.Map{
			"enabled": pulumi.Bool(true),
		},
		"datadogMonitor": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"datadogSLO": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"resources": pulumi.Map{
			"limits": pulumi.Map{
				"cpu":    pulumi.String("100m"),
				"memory": pulumi.String("250Mi"),
			},
			"requests": pulumi.Map{
				"cpu":    pulumi.String("100m"),
				"memory": pulumi.String("250Mi"),
			},
		},
		"installCRDs": pulumi.Bool(true),
		"datadogCRDs": pulumi.Map{
			"crds": pulumi.Map{
				"datadogAgents":   pulumi.Bool(true),
				"datadogMetrics":  pulumi.Bool(true),
				"datadogMonitors": pulumi.Bool(true),
				"datadogSLOs":     pulumi.Bool(true),
			},
		},
	}
}

func (values HelmValues) configureImagePullSecret(secret *corev1.Secret) {
	if secret == nil {
		return
	}

	values["imagePullSecrets"] = pulumi.MapArray{
		pulumi.Map{
			"name": secret.Metadata.Name(),
		},
	}

}

func (values HelmValues) toYAMLPulumiAssetOutput() pulumi.AssetOutput {
	return pulumi.Map(values).ToMapOutput().ApplyT(func(v map[string]interface{}) (pulumi.Asset, error) {
		yamlValues, err := yaml.Marshal(v)
		if err != nil {
			return nil, err
		}
		return pulumi.NewStringAsset(string(yamlValues)), nil
	}).(pulumi.AssetOutput)

}
