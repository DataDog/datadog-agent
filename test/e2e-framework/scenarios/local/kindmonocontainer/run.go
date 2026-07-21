// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kindmonocontainer provisions a local kind cluster and deploys the Datadog
// node agent (DaemonSet) and cluster agent (Deployment) as raw Kubernetes manifests
// without the Helm chart.
package kindmonocontainer

import (
	_ "embed"
	"fmt"
	"strings"

	kubernetesProvider "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	k8syaml "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	agentComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
)

const (
	clusterAgentLeaderElectionName = "datadog-leader-election"
	localNginxWorkerProcesses      = "1"
)

//go:embed rbac.yaml
var rbacYAML string

//go:embed cluster_agent_service.yaml
var clusterAgentServiceYAML string

func Run(ctx *pulumi.Context) error {
	localEnv, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	if err := validateLocalAgentImageConfig(&localEnv); err != nil {
		return err
	}

	cluster, err := kubeComp.NewLocalKindClusterWithConfig(&localEnv, "kind", localEnv.KubernetesVersion(), kubeComp.KindConfigFlags{
		WorkerNodes: []kubeComp.KindWorkerNode{{}, {}, {}, {}},
	})
	if err != nil {
		return err
	}
	if err := cluster.Export(ctx, nil); err != nil {
		return err
	}

	if localEnv.InitOnly() {
		return nil
	}

	kubeProvider, err := kubernetesProvider.NewProvider(ctx, localEnv.CommonNamer().ResourceName("k8s-provider"), &kubernetesProvider.ProviderArgs{
		Kubeconfig:            cluster.KubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
		DeleteUnreachable:     pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}
	providerOpt := pulumi.Provider(kubeProvider)

	if localEnv.AgentDeploy() {
		// Generate auth token shared between node agent and cluster agent.
		// Matches the pattern in components/datadog/agent/kubernetes_helm.go.
		clusterAgentToken, err := random.NewRandomString(ctx, "cluster-agent-token", &random.RandomStringArgs{
			Lower:   pulumi.Bool(true),
			Upper:   pulumi.Bool(true),
			Length:  pulumi.Int(32),
			Numeric: pulumi.Bool(false),
			Special: pulumi.Bool(false),
		}, localEnv.WithProviders(config.ProviderRandom))
		if err != nil {
			return err
		}

		var fakeIntake *fakeintakeComp.Fakeintake
		if localEnv.AgentUseFakeintake() {
			fakeIntake, err = fakeintakeComp.NewLocalDockerFakeintake(&localEnv, "fakeintake")
			if err != nil {
				return err
			}
			if err := fakeIntake.Export(ctx, nil); err != nil {
				return err
			}
		}

		clusterName := ctx.Stack()

		agentImage := agentComp.DockerAgentFullImagePath(&localEnv)
		clusterAgentImage := agentComp.DockerClusterAgentFullImagePath(&localEnv)

		var imgPullSecret *corev1.Secret
		if localEnv.ImagePullRegistry() != "" {
			imgPullSecret, err = utils.NewImagePullSecret(&localEnv, "default", providerOpt)
			if err != nil {
				return err
			}
		}

		if _, err := corev1.NewSecret(ctx, "datadog-secret", &corev1.SecretArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("datadog-secret"),
				Namespace: pulumi.String("default"),
			},
			StringData: pulumi.StringMap{
				"api-key": localEnv.AgentAPIKey(),
			},
		}, providerOpt); err != nil {
			return err
		}

		rbacResources, err := deployRBAC(ctx, providerOpt)
		if err != nil {
			return err
		}

		clusterAgentSvc, err := deployClusterAgentService(ctx, providerOpt)
		if err != nil {
			return err
		}

		clusterAgentDeploy, err := deployClusterAgent(ctx, clusterName, clusterAgentImage, clusterAgentToken.Result, fakeIntake, imgPullSecret, providerOpt, pulumi.DependsOn([]pulumi.Resource{rbacResources, clusterAgentSvc}))
		if err != nil {
			return err
		}

		if err := deployNodeAgentDaemonSet(ctx, clusterName, agentImage, clusterAgentToken.Result, fakeIntake, imgPullSecret, providerOpt, pulumi.DependsOn([]pulumi.Resource{rbacResources, clusterAgentSvc, clusterAgentDeploy})); err != nil {
			return err
		}
	}

	if localEnv.TestingWorkloadDeploy() {
		if _, err := nginx.K8sAppDefinitionWithOptions(&localEnv, kubeProvider, "workload-nginx", 80, "", false, []nginx.K8sAppOption{
			// Local clusters can expose the host CPU count inside pods. The nginx
			// workload uses worker_processes auto by default, which can start one
			// worker per local CPU and exceed the 32Mi limit before becoming ready.
			nginx.WithWorkerProcesses(localNginxWorkerProcesses),
		}, providerOpt); err != nil {
			return err
		}
		if _, err := redis.K8sAppDefinition(&localEnv, kubeProvider, "workload-redis", false, providerOpt); err != nil {
			return err
		}
		if _, err := cpustress.K8sAppDefinition(&localEnv, kubeProvider, "workload-cpustress"); err != nil {
			return err
		}
	}

	return nil
}

func validateLocalAgentImageConfig(e config.Env) error {
	if !e.AgentDeploy() || e.InitOnly() || e.PipelineID() == "" || e.CommitSHA() == "" {
		return nil
	}

	var missing []string
	if e.AgentFullImagePath() == "" {
		missing = append(missing, "ddagent:"+config.DDAgentFullImagePathParamName)
	}
	if e.ClusterAgentFullImagePath() == "" {
		missing = append(missing, "ddagent:"+config.DDClusterAgentFullImagePathParamName)
	}
	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("local/kindmonocontainer cannot use ddagent:%s and ddagent:%s without %s; local environments do not have an internal CI image registry, so set explicit full image paths instead",
		config.DDAgentPipelineID,
		config.DDAgentCommitSHA,
		strings.Join(missing, " and "),
	)
}

func deployRBAC(ctx *pulumi.Context, opts ...pulumi.ResourceOption) (*k8syaml.ConfigGroup, error) {
	return k8syaml.NewConfigGroup(ctx, "datadog-rbac", &k8syaml.ConfigGroupArgs{
		YAML: []string{rbacYAML},
	}, opts...)
}

func deployClusterAgentService(ctx *pulumi.Context, opts ...pulumi.ResourceOption) (*k8syaml.ConfigGroup, error) {
	return k8syaml.NewConfigGroup(ctx, "datadog-ca-svc", &k8syaml.ConfigGroupArgs{
		YAML: []string{clusterAgentServiceYAML},
	}, opts...)
}

func deployClusterAgent(ctx *pulumi.Context, clusterName, image string, authToken pulumi.StringOutput, fakeIntake *fakeintakeComp.Fakeintake, imgPullSecret *corev1.Secret, opts ...pulumi.ResourceOption) (*appsv1.Deployment, error) {
	caEnv := corev1.EnvVarArray{
		&corev1.EnvVarArgs{Name: pulumi.String("DD_HEALTH_PORT"), Value: pulumi.String("5556")},
		&corev1.EnvVarArgs{
			Name: pulumi.String("DD_API_KEY"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name: pulumi.String("datadog-secret"),
					Key:  pulumi.String("api-key"),
				},
			},
		},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_CLUSTER_NAME"), Value: pulumi.String(clusterName)},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_CLUSTER_CHECKS_ENABLED"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_EXTRA_CONFIG_PROVIDERS"), Value: pulumi.String("kube_endpoints kube_services")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_EXTRA_LISTENERS"), Value: pulumi.String("kube_endpoints kube_services")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_LEADER_ELECTION"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_LEADER_LEASE_NAME"), Value: pulumi.String(clusterAgentLeaderElectionName)},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_LEADER_LEASE_DURATION"), Value: pulumi.String("15")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME"), Value: pulumi.String("datadog-cluster-agent")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_CLUSTER_AGENT_AUTH_TOKEN"), Value: authToken},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_KUBE_RESOURCES_NAMESPACE"), Value: pulumi.String("default")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_COLLECT_KUBERNETES_EVENTS"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_ORCHESTRATOR_EXPLORER_ENABLED"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_ORCHESTRATOR_EXPLORER_CONTAINER_SCRUBBING_ENABLED"), Value: pulumi.String("true")},
	}
	if fakeIntake != nil {
		fiVars, err := fakeintakeEnvVars(fakeIntake)
		if err != nil {
			return nil, err
		}
		caEnv = append(caEnv, fiVars...)
	}

	return appsv1.NewDeployment(ctx, "datadog-ca-deploy", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("datadog-cluster-agent"),
			Namespace: pulumi.String("default"),
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{"app": pulumi.String("datadog-cluster-agent")},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{"app": pulumi.String("datadog-cluster-agent")},
					Name:   pulumi.String("datadog-cluster-agent"),
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: pulumi.String("datadog-cluster-agent"),
					NodeSelector:       pulumi.StringMap{"kubernetes.io/os": pulumi.String("linux")},
					ImagePullSecrets:   imagePullSecrets(imgPullSecret),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:            pulumi.String("cluster-agent"),
							Image:           pulumi.String(image),
							ImagePullPolicy: pulumi.String("IfNotPresent"),
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{Name: pulumi.String("agentport"), ContainerPort: pulumi.Int(5005), Protocol: pulumi.String("TCP")},
								&corev1.ContainerPortArgs{Name: pulumi.String("metricsapi"), ContainerPort: pulumi.Int(8443), Protocol: pulumi.String("TCP")},
							},
							Env: caEnv,
							LivenessProbe: &corev1.ProbeArgs{
								HttpGet:             &corev1.HTTPGetActionArgs{Path: pulumi.String("/live"), Port: pulumi.Int(5556), Scheme: pulumi.String("HTTP")},
								InitialDelaySeconds: pulumi.Int(15),
								PeriodSeconds:       pulumi.Int(15),
								SuccessThreshold:    pulumi.Int(1),
								FailureThreshold:    pulumi.Int(6),
								TimeoutSeconds:      pulumi.Int(5),
							},
							ReadinessProbe: &corev1.ProbeArgs{
								HttpGet:             &corev1.HTTPGetActionArgs{Path: pulumi.String("/ready"), Port: pulumi.Int(5556), Scheme: pulumi.String("HTTP")},
								InitialDelaySeconds: pulumi.Int(15),
								PeriodSeconds:       pulumi.Int(15),
								SuccessThreshold:    pulumi.Int(1),
								FailureThreshold:    pulumi.Int(6),
								TimeoutSeconds:      pulumi.Int(5),
							},
						},
					},
				},
			},
		},
	}, opts...)
}

func deployNodeAgentDaemonSet(ctx *pulumi.Context, clusterName, image string, authToken pulumi.StringOutput, fakeIntake *fakeintakeComp.Fakeintake, imgPullSecret *corev1.Secret, opts ...pulumi.ResourceOption) error {
	env := corev1.EnvVarArray{
		&corev1.EnvVarArgs{
			Name: pulumi.String("DD_API_KEY"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name: pulumi.String("datadog-secret"),
					Key:  pulumi.String("api-key"),
				},
			},
		},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_LEADER_ELECTION"), Value: pulumi.String("false")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_LOGS_ENABLED"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("KUBERNETES"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_HEALTH_PORT"), Value: pulumi.String("5555")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_PROCESS_AGENT_ENABLED"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_ORCHESTRATOR_EXPLORER_ENABLED"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_CLUSTER_AGENT_ENABLED"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME"), Value: pulumi.String("datadog-cluster-agent")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_CLUSTER_AGENT_AUTH_TOKEN"), Value: authToken},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_KUBE_RESOURCES_NAMESPACE"), Value: pulumi.String("default")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_CLUSTER_NAME"), Value: pulumi.String(clusterName)},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_TAGS"), Value: pulumi.Sprintf("env:%s", clusterName)},
		&corev1.EnvVarArgs{
			Name: pulumi.String("DD_KUBERNETES_KUBELET_HOST"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				FieldRef: &corev1.ObjectFieldSelectorArgs{FieldPath: pulumi.String("status.hostIP")},
			},
		},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_KUBELET_TLS_VERIFY"), Value: pulumi.String("false")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_EXTRA_CONFIG_PROVIDERS"), Value: pulumi.String("clusterchecks endpointschecks")},
	}

	if fakeIntake != nil {
		fiVars, err := fakeintakeEnvVars(fakeIntake)
		if err != nil {
			return err
		}
		env = append(env, fiVars...)
	}

	_, err := appsv1.NewDaemonSet(ctx, "datadog-agent-ds", &appsv1.DaemonSetArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("datadog-agent"),
			Namespace: pulumi.String("default"),
		},
		Spec: &appsv1.DaemonSetSpecArgs{
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{"app": pulumi.String("datadog-agent")},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{"app": pulumi.String("datadog-agent")},
					Name:   pulumi.String("datadog-agent"),
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: pulumi.String("datadog"),
					ImagePullSecrets:   imagePullSecrets(imgPullSecret),
					Tolerations: corev1.TolerationArray{
						&corev1.TolerationArgs{Operator: pulumi.String("Exists")},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name:     pulumi.String("runtimesocketdir"),
							HostPath: &corev1.HostPathVolumeSourceArgs{Path: pulumi.String("/var/run")},
						},
						&corev1.VolumeArgs{
							Name:     pulumi.String("procdir"),
							HostPath: &corev1.HostPathVolumeSourceArgs{Path: pulumi.String("/proc")},
						},
						&corev1.VolumeArgs{
							Name:     pulumi.String("cgroups"),
							HostPath: &corev1.HostPathVolumeSourceArgs{Path: pulumi.String("/sys/fs/cgroup")},
						},
						&corev1.VolumeArgs{
							Name:     pulumi.String("logpodpath"),
							HostPath: &corev1.HostPathVolumeSourceArgs{Path: pulumi.String("/var/log/pods")},
						},
					},
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:            pulumi.String("datadog-agent"),
							Image:           pulumi.String(image),
							ImagePullPolicy: pulumi.String("Always"),
							Env:             env,
							VolumeMounts: corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:             pulumi.String("runtimesocketdir"),
									MountPath:        pulumi.String("/host/var/run"),
									MountPropagation: pulumi.String("None"),
									ReadOnly:         pulumi.Bool(true),
								},
								&corev1.VolumeMountArgs{
									Name:             pulumi.String("procdir"),
									MountPath:        pulumi.String("/host/proc"),
									MountPropagation: pulumi.String("None"),
									ReadOnly:         pulumi.Bool(true),
								},
								&corev1.VolumeMountArgs{
									Name:             pulumi.String("cgroups"),
									MountPath:        pulumi.String("/host/sys/fs/cgroup"),
									MountPropagation: pulumi.String("None"),
									ReadOnly:         pulumi.Bool(true),
								},
								&corev1.VolumeMountArgs{
									Name:             pulumi.String("logpodpath"),
									MountPath:        pulumi.String("/var/log/pods"),
									MountPropagation: pulumi.String("None"),
									ReadOnly:         pulumi.Bool(true),
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{"memory": pulumi.String("256Mi"), "cpu": pulumi.String("200m")},
								Limits:   pulumi.StringMap{"memory": pulumi.String("256Mi"), "cpu": pulumi.String("200m")},
							},
							LivenessProbe: &corev1.ProbeArgs{
								HttpGet:             &corev1.HTTPGetActionArgs{Path: pulumi.String("/live"), Port: pulumi.Int(5555)},
								InitialDelaySeconds: pulumi.Int(15),
								PeriodSeconds:       pulumi.Int(15),
								TimeoutSeconds:      pulumi.Int(5),
								SuccessThreshold:    pulumi.Int(1),
								FailureThreshold:    pulumi.Int(3),
							},
							ReadinessProbe: &corev1.ProbeArgs{
								HttpGet:             &corev1.HTTPGetActionArgs{Path: pulumi.String("/ready"), Port: pulumi.Int(5555)},
								InitialDelaySeconds: pulumi.Int(15),
								PeriodSeconds:       pulumi.Int(15),
								TimeoutSeconds:      pulumi.Int(5),
								SuccessThreshold:    pulumi.Int(1),
								FailureThreshold:    pulumi.Int(6),
							},
						},
					},
				},
			},
		},
	}, opts...)
	return err
}

// imagePullSecrets returns a LocalObjectReferenceArray referencing the pull
// secret, or nil if no pull secret is configured.
func imagePullSecrets(secret *corev1.Secret) corev1.LocalObjectReferenceArray {
	if secret == nil {
		return nil
	}
	return corev1.LocalObjectReferenceArray{
		&corev1.LocalObjectReferenceArgs{Name: secret.Metadata.Name()},
	}
}

// fakeintakeEnvVars returns the full set of env var overrides needed to route
// all agent traffic (metrics, APM, logs, process, orchestrator, RC) through the
// fakeintake, mirroring the Helm path's configureFakeintake logic.
func fakeintakeEnvVars(fi *fakeintakeComp.Fakeintake) (corev1.EnvVarArray, error) {
	rootJSON, err := fakeintakeComp.RCRootJSON()
	if err != nil {
		return nil, fmt.Errorf("fakeintake rc root json: %w", err)
	}
	return corev1.EnvVarArray{
		&corev1.EnvVarArgs{Name: pulumi.String("DD_DD_URL"), Value: fi.URL},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_APM_DD_URL"), Value: fi.URL},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_PROCESS_CONFIG_PROCESS_DD_URL"), Value: fi.URL},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_LOGS_CONFIG_LOGS_DD_URL"), Value: fi.URL},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL"), Value: fi.URL},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_LOGS_CONFIG_USE_HTTP"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_SKIP_SSL_VALIDATION"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_REMOTE_CONFIGURATION_RC_DD_URL"), Value: fi.URL},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_REMOTE_CONFIGURATION_NO_TLS"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_REMOTE_CONFIGURATION_CONFIG_ROOT"), Value: pulumi.String(rootJSON)},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_REMOTE_CONFIGURATION_DIRECTOR_ROOT"), Value: pulumi.String(rootJSON)},
		&corev1.EnvVarArgs{Name: pulumi.String("DD_REMOTE_CONFIGURATION_REFRESH_INTERVAL"), Value: pulumi.String("5s")},
	}, nil
}
