// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/etcd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/helm"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	kubeHelm "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	HelmVersion = "3.155.1"
)

// HelmInstallationArgs is the set of arguments for creating a new HelmInstallation component
type HelmInstallationArgs struct {
	// KubeProvider is the Kubernetes provider to use
	KubeProvider *kubernetes.Provider
	// Namespace is the namespace in which to install the agent
	Namespace string
	// ChartPath is the chart name or local chart path.
	ChartPath string
	// RepoURL is the Helm repository URL to use for the remote chart installation.
	RepoURL string
	// ValuesYAML is used to provide installation-specific values
	ValuesYAML pulumi.AssetOrArchiveArray
	// Fakeintake is used to configure the agent to send data to a fake intake
	Fakeintake *fakeintake.Fakeintake
	// DeployWindows is used to deploy the Windows agent
	DeployWindows bool
	// AgentFullImagePath is used to specify the full image path for the agent
	AgentFullImagePath string
	// ClusterAgentFullImagePath is used to specify the full image path for the cluster agent
	ClusterAgentFullImagePath string
	// DisableLogsContainerCollectAll is used to disable the collection of logs from all containers by default
	DisableLogsContainerCollectAll bool
	// DualShipping is used to disable dual-shipping
	DualShipping bool
	// OTelAgent is used to deploy the OTel agent instead of the classic agent
	OTelAgent bool
	// OTelAgentGateway is used to deploy the OTel agent with gateway enabled
	OTelAgentGateway bool
	// OTelConfig is used to provide a custom OTel configuration
	OTelConfig string
	// OTelGatewayConfig is used to provide a custom OTel configuration for the gateway collector
	OTelGatewayConfig string
	// GKEAutopilot is used to enable the GKE Autopilot mode and keep only compatible values
	GKEAutopilot bool
	// FIPS is used to deploy the agent with the FIPS agent image
	FIPS bool
	// JMX is used to deploy the agent with the JMX agent image
	JMX bool
	// WindowsImage is used to use Windows-compatible image (multi-arch with Windows)
	WindowsImage bool
	// TimeoutSeconds is the timeout for Helm operations in seconds (default: 300)
	TimeoutSeconds int
}

type HelmComponent struct {
	pulumi.ResourceState

	LinuxHelmReleaseName   pulumi.StringPtrOutput
	LinuxHelmReleaseStatus kubeHelm.ReleaseStatusOutput

	WindowsHelmReleaseName   pulumi.StringPtrOutput
	WindowsHelmReleaseStatus kubeHelm.ReleaseStatusOutput

	ClusterAgentToken pulumi.StringOutput
}

func NewHelmInstallation(e config.Env, args HelmInstallationArgs, opts ...pulumi.ResourceOption) (*HelmComponent, error) {
	apiKey := e.AgentAPIKey()
	appKey := e.AgentAPPKey()
	baseName := "dda"
	opts = append(opts, pulumi.Providers(args.KubeProvider), e.WithProviders(config.ProviderRandom), pulumi.DeletedWith(args.KubeProvider))

	helmComponent := &HelmComponent{}
	if err := e.Ctx().RegisterComponentResource("dd:agent", "dda", helmComponent, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(helmComponent))

	// Create fixed cluster agent token
	randomClusterAgentToken, err := random.NewRandomString(e.Ctx(), "datadog-cluster-agent-token", &random.RandomStringArgs{
		Lower:   pulumi.Bool(true),
		Upper:   pulumi.Bool(true),
		Length:  pulumi.Int(32),
		Numeric: pulumi.Bool(false),
		Special: pulumi.Bool(false),
	}, opts...)
	if err != nil {
		return nil, err
	}

	helmComponent.ClusterAgentToken = randomClusterAgentToken.Result

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
			Name:      pulumi.Sprintf("%s-datadog-credentials", baseName),
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
	agentImagePath := dockerAgentFullImagePath(e, "", "", args.OTelAgent, args.FIPS, args.JMX, args.WindowsImage)
	if args.AgentFullImagePath != "" {
		agentImagePath = args.AgentFullImagePath
	}
	agentImagePath, agentImageTag := utils.ParseImageReference(agentImagePath)

	clusterAgentImagePath := dockerClusterAgentFullImagePath(e, "", args.FIPS)
	if args.ClusterAgentFullImagePath != "" {
		clusterAgentImagePath = args.ClusterAgentFullImagePath
	}
	clusterAgentImagePath, clusterAgentImageTag := utils.ParseImageReference(clusterAgentImagePath)

	linuxInstallName := baseName + "-linux"
	var values HelmValues

	if args.GKEAutopilot {
		values = buildLinuxHelmValuesAutopilot(baseName, agentImagePath, agentImageTag, clusterAgentImagePath, clusterAgentImageTag, randomClusterAgentToken.Result)
	} else if args.OTelAgentGateway {
		values = buildLinuxHelmValues(baseName, agentImagePath, agentImageTag, clusterAgentImagePath, clusterAgentImageTag, randomClusterAgentToken.Result, !args.DisableLogsContainerCollectAll, false, args.FIPS)
		otelAgentGatewayImagePath := dockerOTelAgentGatewayFullImagePath(e, "", "")
		otelAgentGatewayImagePath, otelAgentGatewayImageTag := utils.ParseImageReference(otelAgentGatewayImagePath)
		values["otelAgentGateway"] = pulumi.Map{
			"enabled": pulumi.Bool(true),
			"image": pulumi.Map{
				"repository":    pulumi.String(otelAgentGatewayImagePath),
				"tag":           pulumi.String(otelAgentGatewayImageTag),
				"doNotCheckTag": pulumi.Bool(true),
			},
		}
	} else {
		values = buildLinuxHelmValues(baseName, agentImagePath, agentImageTag, clusterAgentImagePath, clusterAgentImageTag, randomClusterAgentToken.Result, !args.DisableLogsContainerCollectAll, e.TestingWorkloadDeploy(), args.FIPS)
	}
	values.configureImagePullSecret(imgPullSecret)
	values.configureFakeintake(e, args.Fakeintake, args.DualShipping)

	defaultYAMLValues := values.ToYAMLPulumiAssetOutput()

	var valuesYAML pulumi.AssetOrArchiveArray
	valuesYAML = append(valuesYAML, defaultYAMLValues)
	valuesYAML = append(valuesYAML, args.ValuesYAML...)
	if args.OTelAgentGateway {
		valuesYAML = append(valuesYAML, buildOTelAgentGatewayConfigWithFakeintake(args.OTelGatewayConfig, args.Fakeintake))
	}
	if args.OTelAgent && !args.OTelAgentGateway {
		valuesYAML = append(valuesYAML, buildOTelConfigWithFakeintake(args.OTelConfig, args.Fakeintake))
	}

	// Read and merge custom helm config if provided
	if helmConfig := e.AgentHelmConfig(); helmConfig != "" {
		customHelm, err := os.ReadFile(helmConfig)
		if err != nil {
			return nil, err
		}
		config := pulumi.NewStringAsset(string(customHelm))
		valuesYAML = append(valuesYAML, config)
	}

	linux, err := helm.NewInstallation(e, helm.InstallArgs{
		RepoURL:        args.RepoURL,
		ChartName:      args.ChartPath,
		InstallName:    linuxInstallName,
		Namespace:      args.Namespace,
		ValuesYAML:     valuesYAML,
		Version:        pulumi.String(HelmVersion),
		TimeoutSeconds: args.TimeoutSeconds,
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

	if args.DeployWindows {
		values := buildWindowsHelmValues(baseName, agentImagePath, agentImageTag, clusterAgentImagePath, clusterAgentImageTag)
		values.configureImagePullSecret(imgPullSecret)
		values.configureFakeintake(e, args.Fakeintake, args.DualShipping)
		defaultYAMLValues := values.ToYAMLPulumiAssetOutput()

		var windowsValuesYAML pulumi.AssetOrArchiveArray
		windowsValuesYAML = append(windowsValuesYAML, defaultYAMLValues)
		windowsValuesYAML = append(windowsValuesYAML, args.ValuesYAML...)

		windowsInstallName := baseName + "-windows"
		windows, err := helm.NewInstallation(e, helm.InstallArgs{
			RepoURL:     args.RepoURL,
			ChartName:   args.ChartPath,
			InstallName: windowsInstallName,
			Namespace:   args.Namespace,
			ValuesYAML:  windowsValuesYAML,
		}, opts...)
		if err != nil {
			return nil, err
		}

		helmComponent.WindowsHelmReleaseName = windows.Name
		helmComponent.WindowsHelmReleaseStatus = windows.Status

		maps.Copy(resourceOutputs, pulumi.Map{
			"windowsHelmReleaseName":   windows.Name,
			"windowsHelmReleaseStatus": windows.Status,
		})
	}

	if err := e.Ctx().RegisterResourceOutputs(helmComponent, resourceOutputs); err != nil {
		return nil, err
	}

	return helmComponent, nil
}

type HelmValues pulumi.Map

func buildLinuxHelmValues(baseName, agentImagePath, agentImageTag, clusterAgentImagePath, clusterAgentImageTag string, clusterAgentToken pulumi.StringInput, logsContainerCollectAll bool, testingWorkloadsEnabled bool, isFIPS bool) HelmValues {
	var containerRegistry, imageName string
	isAutoscaling := true
	if isFIPS {
		isAutoscaling = false
	}
	if strings.Contains(agentImagePath, "/") {
		agentImageElem := strings.Split(agentImagePath, "/")
		containerRegistry = strings.Join(agentImageElem[:len(agentImageElem)-1], "/")
		imageName = agentImageElem[len(agentImageElem)-1]
	} else {
		containerRegistry = ""
		imageName = agentImagePath
	}
	helmValues := HelmValues{
		"datadog": pulumi.Map{
			"apiKeyExistingSecret":   pulumi.String(baseName + "-datadog-credentials"),
			"appKeyExistingSecret":   pulumi.String(baseName + "-datadog-credentials"),
			"leaderElectionResource": pulumi.String(""),
			"checksCardinality":      pulumi.String("high"),
			"namespaceLabelsAsTags": pulumi.Map{
				"related_team": pulumi.String("team"),
			},
			"namespaceAnnotationsAsTags": pulumi.Map{
				"related_email": pulumi.String("email"), // should be overridden by kubernetesResourcesAnnotationsAsTags
			},
			"kubernetesResourcesAnnotationsAsTags": pulumi.Map{
				"deployments.apps": pulumi.Map{"x-sub-team": pulumi.String("sub-team")},
				"pods":             pulumi.Map{"x-parent-name": pulumi.String("parent-name")},
				"namespaces":       pulumi.Map{"related_email": pulumi.String("mail")},
			},
			"kubernetesResourcesLabelsAsTags": pulumi.Map{
				"deployments.apps": pulumi.Map{"x-team": pulumi.String("team")},
				"pods":             pulumi.Map{"x-parent-type": pulumi.String("domain")},
				"namespaces":       pulumi.Map{"related_org": pulumi.String("org")},
				"nodes": pulumi.Map{
					"kubernetes.io/os":                  pulumi.String("os"),
					"kubernetes.io/arch":                pulumi.String("arch"),
					"eks.amazonaws.com/nodegroup-image": pulumi.String("nodegroup-image"),
				},
			},
			"originDetectionUnified": pulumi.Map{
				"enabled": pulumi.Bool(true),
			},
			"logs": pulumi.Map{
				"enabled":             pulumi.Bool(true),
				"containerCollectAll": pulumi.Bool(logsContainerCollectAll),
			},
			"dogstatsd": pulumi.Map{
				"originDetection": pulumi.Bool(true),
				"tagCardinality":  pulumi.String("high"),
				"useHostPort":     pulumi.Bool(true),
			},
			"apm": pulumi.Map{
				"portEnabled": pulumi.Bool(true),
				"instrumentation": pulumi.Map{
					"enabled": pulumi.Bool(true),
					"enabledNamespaces": pulumi.Array{
						pulumi.String("workload-mutated-lib-injection"),
					},
					"language_detection": pulumi.Map{
						"enabled": pulumi.Bool(true),
					},
				},
			},
			"processAgent": pulumi.Map{
				"processCollection": pulumi.Bool(true),
			},
			"helmCheck": pulumi.Map{
				"enabled": pulumi.Bool(true),
			},
			"kubeStateMetricsCore": pulumi.Map{
				"enabled":                pulumi.Bool(true),
				"collectVpaMetrics":      pulumi.Bool(true),
				"useClusterCheckRunners": pulumi.Bool(true),
				"collectCrMetrics": pulumi.MapArray{
					pulumi.Map{
						"groupVersionKind": pulumi.StringMap{
							"group":   pulumi.String("datadoghq.com"),
							"kind":    pulumi.String("DatadogMetric"),
							"version": pulumi.String("v1alpha1"),
						},
						"commonLabels": pulumi.StringMap{
							"cr_type": pulumi.String("ddm"),
						},
						"labelsFromPath": pulumi.StringArrayMap{
							"ddm_namespace": pulumi.ToStringArray([]string{"metadata", "namespace"}),
							"ddm_name":      pulumi.ToStringArray([]string{"metadata", "name"}),
						},
						"metrics": pulumi.MapArray{
							pulumi.Map{
								"name": pulumi.String("ddm_value"),
								"help": pulumi.String("DatadogMetric value"),
								"each": pulumi.Map{
									"type": pulumi.String("gauge"),
									"gauge": pulumi.StringArrayMap{
										"path": pulumi.ToStringArray([]string{"status", "currentValue"}),
									},
								},
							},
						},
					},
				},
				"tags": pulumi.ToStringArray([]string{"kube_instance_tag:static"}),
			},
			"prometheusScrape": pulumi.Map{
				"enabled": pulumi.Bool(true),
			},
			"sbom": pulumi.Map{
				"host": pulumi.Map{
					"enabled": pulumi.Bool(true),
				},
				"containerImage": pulumi.Map{
					"enabled":                   pulumi.Bool(true),
					"uncompressedLayersSupport": pulumi.Bool(true),
				},
			},
			// The fake intake keeps payloads only for a hardcoded period of 15 minutes.
			// https://github.com/DataDog/datadog-agent/blob/34922393ce47261da9835d7bf62fb5e090e5fa55/test/fakeintake/server/server.go#L81
			// So, we need `container_image` and `sbom` checks to resubmit their payloads more frequently than that.
			"confd": pulumi.StringMap{
				"container_image.yaml": pulumi.String(utils.JSONMustMarshal(map[string]any{
					"ad_identifiers": []string{"_container_image"},
					"init_config":    map[string]any{},
					"instances": []map[string]any{
						{
							"periodic_refresh_seconds": 60, // To have at least one refresh per test
						},
					},
				})),
				"sbom.yaml": pulumi.String(utils.JSONMustMarshal(map[string]any{
					"ad_identifiers": []string{"_sbom"},
					"init_config":    map[string]any{},
					"instances": []map[string]any{
						{
							"periodic_refresh_seconds": 60, // To have at least one refresh per test
						},
					},
				})),
			},
			"env": pulumi.StringMapArray{
				pulumi.StringMap{
					"name":  pulumi.String("DD_EC2_METADATA_TIMEOUT"),
					"value": pulumi.String("5000"), // Unit is ms
				},
				pulumi.StringMap{
					"name":  pulumi.String("DD_TELEMETRY_ENABLED"),
					"value": pulumi.String("true"),
				},
				pulumi.StringMap{
					"name":  pulumi.String("DD_TELEMETRY_CHECKS"),
					"value": pulumi.String("*"),
				},
			},
			"autoscaling": pulumi.Map{
				"workload": pulumi.Map{
					// Autoscaling is not possible for FIPS as it requires remote-config and this is not available in FIPS agent.
					"enabled": pulumi.Bool(isAutoscaling),
				},
			},
		},
		"agents": pulumi.Map{
			"image": pulumi.Map{
				"repository":    pulumi.String(agentImagePath),
				"tag":           pulumi.String(agentImageTag),
				"doNotCheckTag": pulumi.Bool(true),
			},
			"priorityClassCreate": pulumi.Bool(true),
			"podAnnotations": pulumi.StringMap{
				"ad.datadoghq.com/agent.checks": pulumi.String(utils.JSONMustMarshal(
					map[string]any{
						"openmetrics": map[string]any{
							"init_config": map[string]any{},
							"instances": []map[string]any{
								{
									"openmetrics_endpoint": "http://localhost:6000/telemetry",
									"namespace":            "datadog.agent",
									"metrics": []string{
										".*",
									},
								},
							},
						},
					}),
				),
			},
			"containers": pulumi.Map{
				"agent": pulumi.Map{
					"env": pulumi.StringMapArray{
						pulumi.StringMap{
							// TODO: remove this environment variable override once a retry mechanism is added to the language detection client
							//
							// the refresh period is reduced to 1 minute because the language detection client doesn't implement a retry mechanism
							// if the cluster agent is not available when the client tries to send the first detected language, the language will only
							// be sent again after 20 minutes (default refresh period). This causes E2E to fail since it only waits 5 minutes.
							"name":  pulumi.String("DD_LANGUAGE_DETECTION_REPORTING_REFRESH_PERIOD"),
							"value": pulumi.String("1m"),
						},
					},
					"resources": pulumi.StringMapMap{
						"requests": pulumi.StringMap{
							"cpu":    pulumi.String("400m"),
							"memory": pulumi.String("500Mi"),
						},
						"limits": pulumi.StringMap{
							"cpu":    pulumi.String("1000m"),
							"memory": pulumi.String("700Mi"),
						},
					},
				},
				"processAgent": pulumi.Map{
					"resources": pulumi.StringMapMap{
						"requests": pulumi.StringMap{
							"cpu":    pulumi.String("50m"),
							"memory": pulumi.String("150Mi"),
						},
						"limits": pulumi.StringMap{
							"cpu":    pulumi.String("200m"),
							"memory": pulumi.String("200Mi"),
						},
					},
				},
				"traceAgent": pulumi.Map{
					"resources": pulumi.StringMapMap{
						"requests": pulumi.StringMap{
							"cpu":    pulumi.String("10m"),
							"memory": pulumi.String("120Mi"),
						},
						"limits": pulumi.StringMap{
							"cpu":    pulumi.String("200m"),
							"memory": pulumi.String("200Mi"),
						},
					},
				},
			},
		},
		"clusterAgent": pulumi.Map{
			"enabled": pulumi.Bool(true),
			"image": pulumi.Map{
				"repository":    pulumi.String(clusterAgentImagePath),
				"tag":           pulumi.String(clusterAgentImageTag),
				"doNotCheckTag": pulumi.Bool(true),
			},
			"replicas": pulumi.Int(2),
			"metricsProvider": pulumi.Map{
				"enabled":           pulumi.Bool(true),
				"useDatadogMetrics": pulumi.Bool(true),
			},
			"token": clusterAgentToken,
			"admissionController": pulumi.Map{
				"agentSidecarInjection": pulumi.Map{
					"enabled":           pulumi.Bool(true),
					"provider":          pulumi.String("fargate"),
					"containerRegistry": pulumi.String(containerRegistry),
					"imageName":         pulumi.String(imageName),
					"imageTag":          pulumi.String(agentImageTag),
				},
			},
			"resources": pulumi.StringMapMap{
				"requests": pulumi.StringMap{
					"cpu":    pulumi.String("50m"),
					"memory": pulumi.String("150Mi"),
				},
				"limits": pulumi.StringMap{
					"cpu":    pulumi.String("200m"),
					"memory": pulumi.String("200Mi"),
				},
			},
			"env": pulumi.StringMapArray{
				// These options are disabled by default and not exposed in the
				// Helm chart yet, so we need to set the env.
				pulumi.StringMap{
					"name":  pulumi.String("DD_CLUSTER_CHECKS_CRD_COLLECTION"),
					"value": pulumi.String("true"),
				},
				pulumi.StringMap{
					"name":  pulumi.String("DD_EC2_METADATA_TIMEOUT"),
					"value": pulumi.String("5000"), // Unit is ms
				},
				// These options are disabled by default and not exposed in the
				// Helm chart yet, so we need to set the env.
				pulumi.StringMap{
					"name":  pulumi.String("DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_INJECT_AUTO_DETECTED_LIBRARIES"),
					"value": pulumi.String("true"),
				},
				pulumi.StringMap{
					"name":  pulumi.String("DD_ADMISSION_CONTROLLER_AGENT_SIDECAR_KUBELET_API_LOGGING_ENABLED"),
					"value": pulumi.String("true"),
				},
				// Use NoOpResolver for injector version in e2e (avoids gradual rollout / bucket tag resolver).
				// Override in test Helm values (clusterAgent.env) to "true" to exercise gradual rollout.
				pulumi.StringMap{
					"name":  pulumi.String("DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_GRADUAL_ROLLOUT_ENABLED"),
					"value": pulumi.String("false"),
				},
			},
		},
		"clusterChecksRunner": pulumi.Map{
			"enabled": pulumi.Bool(true),
			"image": pulumi.Map{
				"repository":    pulumi.String(agentImagePath),
				"tag":           pulumi.String(agentImageTag),
				"doNotCheckTag": pulumi.Bool(true),
			},
			"env": pulumi.StringMapArray{
				pulumi.StringMap{
					"name":  pulumi.String("DD_CLC_RUNNER_REMOTE_TAGGER_ENABLED"),
					"value": pulumi.String("true"),
				},
				// namespace labels as tags are removed here ƒrom the cluster check runner to
				// be able to test that it can get the namespace labels from the cluster tagger
				// via the remote tagger
				pulumi.StringMap{
					"name":  pulumi.String("DD_KUBERNETES_NAMESPACE_LABELS_AS_TAGS"),
					"value": pulumi.JSONMarshal(map[string]interface{}{}),
				},
			},
			"resources": pulumi.StringMapMap{
				"requests": pulumi.StringMap{
					"cpu":    pulumi.String("20m"),
					"memory": pulumi.String("300Mi"),
				},
				"limits": pulumi.StringMap{
					"cpu":    pulumi.String("200m"),
					"memory": pulumi.String("400Mi"),
				},
			},
		},
	}

	if testingWorkloadsEnabled {
		// This is only needed when both etcd and the Prometheus app (that the
		// check in etcd targets) are deployed.
		//
		// "useConfigMap" and "customAgentConfig" are used to configure the
		// agent to get check configurations from etcd. "config_providers"
		// cannot be configured via ENV, so we need to use a ConfigMap.

		agents := helmValues["agents"].(pulumi.Map)

		agents["useConfigMap"] = pulumi.Bool(true)
		agents["customAgentConfig"] = pulumi.Map{
			"config_providers": pulumi.Array{
				pulumi.Map{
					"name":         pulumi.String("etcd"),
					"polling":      pulumi.Bool(true),
					"template_dir": pulumi.String("/datadog/check_configs"),
					// This relies on a service exposed by the etcd app
					"template_url": pulumi.String(
						fmt.Sprintf("http://%s.%s.svc.cluster.local:2379", etcd.ServiceName, etcd.Namespace),
					),
				},
			},
		}
	}

	return helmValues
}

func buildLinuxHelmValuesAutopilot(baseName, agentImagePath, agentImageTag, clusterAgentImagePath, clusterAgentImageTag string, clusterAgentToken pulumi.StringInput) HelmValues {
	return HelmValues{
		"providers": pulumi.Map{
			"gke": pulumi.Map{
				"autopilot": pulumi.Bool(true),
			},
		},
		"datadog": pulumi.Map{
			"apiKeyExistingSecret": pulumi.String(baseName + "-datadog-credentials"),
			"appKeyExistingSecret": pulumi.String(baseName + "-datadog-credentials"),
		},
		"clusterAgent": pulumi.Map{
			"enabled": pulumi.Bool(true),
			"metricsProvider": pulumi.Map{
				"enabled":           pulumi.Bool(true),
				"useDatadogMetrics": pulumi.Bool(true),
			},
			"image": pulumi.Map{
				"repository":    pulumi.String(clusterAgentImagePath),
				"tag":           pulumi.String(clusterAgentImageTag),
				"doNotCheckTag": pulumi.Bool(true),
			},
			"token": clusterAgentToken,
		},
		"agents": pulumi.Map{
			"image": pulumi.Map{
				"repository":    pulumi.String(agentImagePath),
				"tag":           pulumi.String(agentImageTag),
				"doNotCheckTag": pulumi.Bool(true),
			},
		},
		"clusterChecksRunner": pulumi.Map{
			"enabled": pulumi.Bool(false),
			"image": pulumi.Map{
				"repository":    pulumi.String(agentImagePath),
				"tag":           pulumi.String(agentImageTag),
				"doNotCheckTag": pulumi.Bool(true),
			},
			"env": pulumi.StringMapArray{
				pulumi.StringMap{
					"name":  pulumi.String("DD_CLC_RUNNER_REMOTE_TAGGER_ENABLED"),
					"value": pulumi.String("true"),
				},
			},
			"resources": pulumi.StringMapMap{
				"requests": pulumi.StringMap{
					"cpu":    pulumi.String("20m"),
					"memory": pulumi.String("300Mi"),
				},
				"limits": pulumi.StringMap{
					"cpu":    pulumi.String("200m"),
					"memory": pulumi.String("400Mi"),
				},
			},
		},
	}
}

// BuildOpenShiftHelmValues returns Helm values for deploying the agent on OpenShift clusters.
func BuildOpenShiftHelmValues() HelmValues {
	return HelmValues{
		"datadog": pulumi.Map{
			"kubelet": pulumi.Map{
				"tlsVerify": pulumi.Bool(false),
			},
			// https://docs.datadoghq.com/containers/troubleshooting/admission-controller/?tab=helm#openshift
			"apm": pulumi.Map{
				"portEnabled": pulumi.Bool(true),
			},
			"sbom": pulumi.Map{
				"containerImage": pulumi.Map{
					"enabled":             pulumi.Bool(true),
					"overlayFSDirectScan": pulumi.Bool(true),
				},
			},
			"criSocketPath": pulumi.String("/var/run/crio/crio.sock"),
			"useHostPID":    pulumi.Bool(true),
			"originDetectionUnified": pulumi.Map{
				"enabled": pulumi.Bool(true),
			},
			"dogstatsd": pulumi.Map{
				"originDetection": pulumi.Bool(true),
				"tagCardinality":  pulumi.String("high"),
			},
		},
		"agents": pulumi.Map{
			"enabled": pulumi.Bool(true),
			"tolerations": pulumi.MapArray{
				// Deploy Agents on master nodes
				pulumi.Map{
					"effect":   pulumi.String("NoSchedule"),
					"key":      pulumi.String("node-role.kubernetes.io/master"),
					"operator": pulumi.String("Exists"),
				},
				// Deploy Agents on infra nodes
				pulumi.Map{
					"effect":   pulumi.String("NoSchedule"),
					"key":      pulumi.String("node-role.kubernetes.io/infra"),
					"operator": pulumi.String("Exists"),
				},
				// Tolerate disk pressure
				pulumi.Map{
					"effect":   pulumi.String("NoSchedule"),
					"key":      pulumi.String("node.kubernetes.io/disk-pressure"),
					"operator": pulumi.String("Exists"),
				},
			},
			"useHostNetwork": pulumi.Bool(true),
			"replicas":       pulumi.Int(1),
			"podSecurity": pulumi.Map{
				"securityContextConstraints": pulumi.Map{
					"create": pulumi.Bool(true),
				},
			},
			"volumeMounts": pulumi.MapArray{
				pulumi.Map{
					"name":      pulumi.String("trivycache"),
					"mountPath": pulumi.String("/root/.cache/trivy"),
				},
				pulumi.Map{
					"name":      pulumi.String("imageoverlay"),
					"mountPath": pulumi.String("/var/lib/containers/storage"),
				},
			},
			"volumes": pulumi.MapArray{
				pulumi.Map{
					"name":     pulumi.String("trivycache"),
					"emptyDir": pulumi.Map{},
				},
				pulumi.Map{
					"name": pulumi.String("imageoverlay"),
					"hostPath": pulumi.Map{
						"path": pulumi.String("/var/lib/containers/storage"),
					},
				},
			},
		},
		"clusterAgent": pulumi.Map{
			"resources": pulumi.StringMapMap{
				"limits": pulumi.StringMap{
					"cpu":    pulumi.String("300m"),
					"memory": pulumi.String("400Mi"),
				},
				"requests": pulumi.StringMap{
					"cpu":    pulumi.String("150m"),
					"memory": pulumi.String("300Mi"),
				},
			},
			"enabled": pulumi.Bool(true),
			"podSecurity": pulumi.Map{
				"securityContextConstraints": pulumi.Map{
					"create": pulumi.Bool(true),
				},
			},
		},
	}
}

func buildWindowsHelmValues(baseName string, agentImagePath, agentImageTag, _, _ string) HelmValues {
	return HelmValues{
		"targetSystem": pulumi.String("windows"),
		"datadog": pulumi.Map{
			"apiKeyExistingSecret": pulumi.String(baseName + "-datadog-credentials"),
			"appKeyExistingSecret": pulumi.String(baseName + "-datadog-credentials"),
			"checksCardinality":    pulumi.String("high"),
			"logs": pulumi.Map{
				"enabled":             pulumi.Bool(true),
				"containerCollectAll": pulumi.Bool(true),
			},
			"dogstatsd": pulumi.Map{
				"originDetection": pulumi.Bool(true),
				"tagCardinality":  pulumi.String("high"),
				"useHostPort":     pulumi.Bool(true),
			},
			"apm": pulumi.Map{
				"portEnabled": pulumi.Bool(true),
			},
			"processAgent": pulumi.Map{
				"processCollection": pulumi.Bool(true),
			},
			"prometheusScrape": pulumi.Map{
				"enabled": pulumi.Bool(true),
			},
		},
		"agents": pulumi.Map{
			"image": pulumi.Map{
				"repository":    pulumi.String(agentImagePath),
				"tag":           pulumi.String(agentImageTag),
				"doNotCheckTag": pulumi.Bool(true),
			},
			"nodeSelector": pulumi.Map{
				"kubernetes.io/arch": pulumi.String("amd64"),
			},
		},
		// Make the Windows node agents target the Linux cluster agent
		"clusterAgent": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"existingClusterAgent": pulumi.Map{
			"join":                 pulumi.Bool(true),
			"serviceName":          pulumi.String(baseName + "-linux-datadog-cluster-agent"),
			"tokenSecretName":      pulumi.String(baseName + "-linux-datadog-cluster-agent"),
			"clusterchecksEnabled": pulumi.Bool(false),
		},
		"clusterChecksRunner": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
	}
}

func (values HelmValues) configureImagePullSecret(secret *corev1.Secret) {
	if secret == nil {
		return
	}

	for _, section := range []string{"agents", "clusterAgent", "clusterChecksRunner", "otelAgentGateway"} {
		if _, ok := values[section].(pulumi.Map); !ok {
			continue
		}
		if _, found := values[section].(pulumi.Map)["image"]; found {
			values[section].(pulumi.Map)["image"].(pulumi.Map)["pullSecrets"] = pulumi.MapArray{
				pulumi.Map{
					"name": secret.Metadata.Name(),
				},
			}
		}
	}
}

func (values HelmValues) configureFakeintake(e config.Env, fakeintake *fakeintake.Fakeintake, dualShipping bool) {
	if fakeintake == nil {
		return
	}

	var endpointsEnvVar pulumi.StringMapArray
	if dualShipping {
		useSSL := fakeintake.Scheme.ApplyT(func(scheme string) bool {
			if scheme != "https" {
				e.Ctx().Log.Warn("Fakeintake is used in HTTP with dual-shipping, some endpoints will not work", nil)
			}

			return scheme == "https"
		}).(pulumi.BoolOutput)

		endpointsEnvVar = pulumi.StringMapArray{
			pulumi.StringMap{
				"name":  pulumi.String("DD_SKIP_SSL_VALIDATION"),
				"value": pulumi.String("true"),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION"),
				"value": pulumi.String("true"),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_ADDITIONAL_ENDPOINTS"),
				"value": pulumi.Sprintf(`{"%s": ["FAKEAPIKEY"]}`, fakeintake.URL),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_PROCESS_ADDITIONAL_ENDPOINTS"),
				"value": pulumi.Sprintf(`{"%s": ["FAKEAPIKEY"]}`, fakeintake.URL),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_ADDITIONAL_ENDPOINTS"),
				"value": pulumi.Sprintf(`{"%s": ["FAKEAPIKEY"]}`, fakeintake.URL),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS"),
				"value": pulumi.Sprintf(`[{"host": "%s", "port": %v, "use_ssl": %t}]`, fakeintake.Host, fakeintake.Port, useSSL),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_LOGS_CONFIG_USE_HTTP"),
				"value": pulumi.String("true"),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_CONTAINER_IMAGE_ADDITIONAL_ENDPOINTS"),
				"value": pulumi.Sprintf(`[{"host": "%s", "use_ssl": %t}]`, fakeintake.Host, useSSL),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_CONTAINER_LIFECYCLE_ADDITIONAL_ENDPOINTS"),
				"value": pulumi.Sprintf(`[{"host": "%s", "use_ssl": %t}]`, fakeintake.Host, useSSL),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_SBOM_ADDITIONAL_ENDPOINTS"),
				"value": pulumi.Sprintf(`[{"host": "%s", "use_ssl": %t}]`, fakeintake.Host, useSSL),
			},
		}
	} else {
		endpointsEnvVar = pulumi.StringMapArray{
			pulumi.StringMap{
				"name":  pulumi.String("DD_DD_URL"),
				"value": pulumi.Sprintf("%s", fakeintake.URL),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_PROCESS_CONFIG_PROCESS_DD_URL"),
				"value": pulumi.Sprintf("%s", fakeintake.URL),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_APM_DD_URL"),
				"value": pulumi.Sprintf("%s", fakeintake.URL),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_LOGS_CONFIG_LOGS_DD_URL"),
				"value": pulumi.Sprintf("%s", fakeintake.URL),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL"),
				"value": pulumi.Sprintf("%s", fakeintake.URL),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_SKIP_SSL_VALIDATION"),
				"value": pulumi.String("true"),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION"),
				"value": pulumi.String("true"),
			},
			pulumi.StringMap{
				"name":  pulumi.String("DD_LOGS_CONFIG_USE_HTTP"),
				"value": pulumi.String("true"),
			},
		}
	}

	for _, section := range []string{"datadog", "clusterAgent", "clusterChecksRunner"} {
		if _, ok := values[section].(pulumi.Map); !ok {
			continue
		}

		if _, found := values[section].(pulumi.Map)["env"]; !found {
			values[section].(pulumi.Map)["env"] = endpointsEnvVar
		} else {
			values[section].(pulumi.Map)["env"] = append(values[section].(pulumi.Map)["env"].(pulumi.StringMapArray), endpointsEnvVar...)
		}
	}

	if _, ok := values["clusterAgent"]; ok {
		if _, ok := values["clusterAgent"].(pulumi.Map)["admissionController"]; ok {
			if _, ok := values["clusterAgent"].(pulumi.Map)["admissionController"].(pulumi.Map)["agentSidecarInjection"]; ok {
				if _, ok := values["clusterAgent"].(pulumi.Map)["admissionController"].(pulumi.Map)["agentSidecarInjection"].(pulumi.Map)["profiles"]; !ok {
					values["clusterAgent"].(pulumi.Map)["admissionController"].(pulumi.Map)["agentSidecarInjection"].(pulumi.Map)["profiles"] = pulumi.Array{
						pulumi.Map{
							"env": endpointsEnvVar,
						},
					}
				} else {
					values["clusterAgent"].(pulumi.Map)["admissionController"].(pulumi.Map)["agentSidecarInjection"].(pulumi.Map)["profiles"] =
						append(values["clusterAgent"].(pulumi.Map)["admissionController"].(pulumi.Map)["agentSidecarInjection"].(pulumi.Map)["profiles"].(pulumi.Array),
							pulumi.Map{
								"env": endpointsEnvVar,
							},
						)
				}
			}
		}
	}
}

func (values HelmValues) ToYAMLPulumiAssetOutput() pulumi.AssetOutput {
	return pulumi.Map(values).ToMapOutput().ApplyT(func(v map[string]any) (pulumi.Asset, error) {
		yamlValues, err := yaml.Marshal(v)
		if err != nil {
			return nil, err
		}
		return pulumi.NewStringAsset(string(yamlValues)), nil
	}).(pulumi.AssetOutput)

}

func buildOTelConfigWithFakeintake(otelConfig string, fakeintake *fakeintake.Fakeintake) pulumi.AssetOutput {

	return fakeintake.URL.ApplyT(func(url string) (pulumi.Asset, error) {
		defaultConfig := map[string]any{
			"exporters": map[string]any{
				"datadog": map[string]any{
					"metrics": map[string]any{
						"endpoint": url,
					},
					"traces": map[string]any{
						"endpoint": url,
					},
					"logs": map[string]any{
						"endpoint": url,
					},
				},
			},
		}
		config := map[string]any{}
		if err := yaml.Unmarshal([]byte(otelConfig), &config); err != nil {
			return nil, err
		}
		mergeSlices := false
		mergedConfig := utils.MergeMaps(config, defaultConfig, mergeSlices)
		mergedConfigYAML, err := yaml.Marshal(mergedConfig)
		if err != nil {
			return nil, err
		}
		otelConfigValues := fmt.Sprintf(`
datadog:
  otelCollector:
    config: |
%s
`, utils.IndentMultilineString(string(mergedConfigYAML), 6))
		return pulumi.NewStringAsset(otelConfigValues), nil

	}).(pulumi.AssetOutput)
}

func buildOTelAgentGatewayConfigWithFakeintake(otelConfig string, fakeintake *fakeintake.Fakeintake) pulumi.AssetOutput {
	return fakeintake.URL.ApplyT(func(url string) (pulumi.Asset, error) {
		defaultConfig := map[string]any{
			"exporters": map[string]any{
				"datadog": map[string]any{
					"metrics": map[string]any{
						"endpoint": url,
					},
					"traces": map[string]any{
						"endpoint": url,
					},
					"logs": map[string]any{
						"endpoint": url,
					},
				},
			},
		}
		config := map[string]any{}
		if err := yaml.Unmarshal([]byte(otelConfig), &config); err != nil {
			return nil, err
		}
		mergeSlices := false
		mergedConfig := utils.MergeMaps(config, defaultConfig, mergeSlices)
		mergedConfigYAML, err := yaml.Marshal(mergedConfig)
		if err != nil {
			return nil, err
		}
		otelConfigValues := fmt.Sprintf(`
otelAgentGateway:
  config: |
%s
`, utils.IndentMultilineString(string(mergedConfigYAML), 4))
		return pulumi.NewStringAsset(otelConfigValues), nil

	}).(pulumi.AssetOutput)
}
