// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kindmonocontainer provisions a local kind cluster and deploys the Datadog
// node agent (DaemonSet) and cluster agent (Deployment) as raw Kubernetes manifests
// without the Helm chart.
package kindmonocontainer

import (
	kubernetesProvider "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	agentComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/cpustress"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/etcd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/prometheus"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/tracegen"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
)


func Run(ctx *pulumi.Context) error {
	localEnv, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	cluster, err := kubeComp.NewLocalKindClusterWithConfig(&localEnv, "kind", localEnv.KubernetesVersion(), kubeComp.KindConfigFlags{
		WorkerNodes: []kubeComp.KindWorkerNode{{}, {}, {}},
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

	if !localEnv.AgentDeploy() {
		return nil
	}

	clusterName := ctx.Stack()

	agentImage := agentComp.DockerAgentFullImagePath(&localEnv)
	clusterAgentImage := agentComp.DockerClusterAgentFullImagePath(&localEnv)

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

	if err := deployNodeAgentRBAC(ctx, &localEnv, providerOpt); err != nil {
		return err
	}

	if err := deployClusterAgentRBAC(ctx, &localEnv, providerOpt); err != nil {
		return err
	}

	if err := deployClusterAgent(ctx, clusterName, clusterAgentImage, clusterAgentToken.Result, providerOpt); err != nil {
		return err
	}

	if err := deployNodeAgentDaemonSet(ctx, clusterName, agentImage, clusterAgentToken.Result, fakeIntake, providerOpt); err != nil {
		return err
	}

	if localEnv.TestingWorkloadDeploy() {
		if _, err := nginx.K8sAppDefinition(&localEnv, kubeProvider, "workload-nginx", 80, "", false, providerOpt); err != nil {
			return err
		}
		if _, err := redis.K8sAppDefinition(&localEnv, kubeProvider, "workload-redis", false, providerOpt); err != nil {
			return err
		}
		if _, err := cpustress.K8sAppDefinition(&localEnv, kubeProvider, "workload-cpustress"); err != nil {
			return err
		}
		if _, err := tracegen.K8sAppDefinition(&localEnv, kubeProvider, "workload-tracegen"); err != nil {
			return err
		}
		if _, err := prometheus.K8sAppDefinition(&localEnv, kubeProvider, "workload-prometheus"); err != nil {
			return err
		}
		if _, err := etcd.K8sAppDefinition(&localEnv, kubeProvider); err != nil {
			return err
		}
	}

	return nil
}

func deployNodeAgentRBAC(ctx *pulumi.Context, _ config.Env, providerOpt pulumi.ResourceOption) error {
	sa, err := corev1.NewServiceAccount(ctx, "datadog-sa", &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("datadog"),
			Namespace: pulumi.String("default"),
		},
	}, providerOpt)
	if err != nil {
		return err
	}

	cr, err := rbacv1.NewClusterRole(ctx, "datadog-cr", &rbacv1.ClusterRoleArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("datadog")},
		Rules: rbacv1.PolicyRuleArray{
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{
					pulumi.String("services"), pulumi.String("events"), pulumi.String("endpoints"),
					pulumi.String("pods"), pulumi.String("nodes"), pulumi.String("componentstatuses"),
				},
				Verbs: pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups:     pulumi.StringArray{pulumi.String("")},
				Resources:     pulumi.StringArray{pulumi.String("configmaps")},
				ResourceNames: pulumi.StringArray{pulumi.String("datadogtoken"), pulumi.String("datadog-leader-election")},
				Verbs:         pulumi.StringArray{pulumi.String("get"), pulumi.String("update")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{pulumi.String("configmaps")},
				Verbs:     pulumi.StringArray{pulumi.String("create")},
			},
			&rbacv1.PolicyRuleArgs{
				NonResourceURLs: pulumi.StringArray{pulumi.String("/version"), pulumi.String("/healthz"), pulumi.String("/metrics")},
				Verbs:           pulumi.StringArray{pulumi.String("get")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{
					pulumi.String("nodes/metrics"), pulumi.String("nodes/spec"),
					pulumi.String("nodes/proxy"), pulumi.String("nodes/stats"),
				},
				Verbs: pulumi.StringArray{pulumi.String("get")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{pulumi.String("endpoints")},
				Verbs:     pulumi.StringArray{pulumi.String("get")},
			},
		},
	}, providerOpt)
	if err != nil {
		return err
	}

	_, err = rbacv1.NewClusterRoleBinding(ctx, "datadog-crb", &rbacv1.ClusterRoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("datadog")},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("datadog"),
		},
		Subjects: rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      pulumi.String("datadog"),
				Namespace: pulumi.String("default"),
			},
		},
	}, providerOpt, pulumi.DependsOn([]pulumi.Resource{sa, cr}))
	return err
}

func deployClusterAgentRBAC(ctx *pulumi.Context, _ config.Env, providerOpt pulumi.ResourceOption) error {
	sa, err := corev1.NewServiceAccount(ctx, "datadog-ca-sa", &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("datadog-cluster-agent"),
			Namespace: pulumi.String("default"),
		},
	}, providerOpt)
	if err != nil {
		return err
	}

	cr, err := rbacv1.NewClusterRole(ctx, "datadog-ca-cr", &rbacv1.ClusterRoleArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("datadog-cluster-agent")},
		Rules: rbacv1.PolicyRuleArray{
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{
					pulumi.String("services"), pulumi.String("endpoints"), pulumi.String("pods"),
					pulumi.String("nodes"), pulumi.String("namespaces"), pulumi.String("componentstatuses"),
				},
				Verbs: pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{pulumi.String("events")},
				Verbs:     pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch"), pulumi.String("create")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("autoscaling")},
				Resources: pulumi.StringArray{pulumi.String("horizontalpodautoscalers")},
				Verbs:     pulumi.StringArray{pulumi.String("list"), pulumi.String("watch")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups:     pulumi.StringArray{pulumi.String("")},
				Resources:     pulumi.StringArray{pulumi.String("configmaps")},
				ResourceNames: pulumi.StringArray{pulumi.String("datadogtoken"), pulumi.String("datadog-leader-election"), pulumi.String("datadog-custom-metrics")},
				Verbs:         pulumi.StringArray{pulumi.String("get"), pulumi.String("update")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups:     pulumi.StringArray{pulumi.String("")},
				Resources:     pulumi.StringArray{pulumi.String("configmaps")},
				ResourceNames: pulumi.StringArray{pulumi.String("extension-apiserver-authentication")},
				Verbs:         pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{pulumi.String("configmaps"), pulumi.String("events")},
				Verbs:     pulumi.StringArray{pulumi.String("create")},
			},
			&rbacv1.PolicyRuleArgs{
				NonResourceURLs: pulumi.StringArray{pulumi.String("/version"), pulumi.String("/healthz")},
				Verbs:           pulumi.StringArray{pulumi.String("get")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups:     pulumi.StringArray{pulumi.String("")},
				Resources:     pulumi.StringArray{pulumi.String("namespaces")},
				ResourceNames: pulumi.StringArray{pulumi.String("kube-system")},
				Verbs:         pulumi.StringArray{pulumi.String("get")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups:     pulumi.StringArray{pulumi.String("")},
				Resources:     pulumi.StringArray{pulumi.String("configmaps")},
				ResourceNames: pulumi.StringArray{pulumi.String("datadog-cluster-id")},
				Verbs:         pulumi.StringArray{pulumi.String("create"), pulumi.String("get"), pulumi.String("update")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("apps")},
				Resources: pulumi.StringArray{pulumi.String("deployments"), pulumi.String("replicasets"), pulumi.String("daemonsets")},
				Verbs:     pulumi.StringArray{pulumi.String("list"), pulumi.String("get"), pulumi.String("watch")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("batch")},
				Resources: pulumi.StringArray{pulumi.String("cronjobs"), pulumi.String("jobs")},
				Verbs:     pulumi.StringArray{pulumi.String("list"), pulumi.String("get"), pulumi.String("watch")},
			},
		},
	}, providerOpt)
	if err != nil {
		return err
	}

	crb, err := rbacv1.NewClusterRoleBinding(ctx, "datadog-ca-crb", &rbacv1.ClusterRoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("datadog-cluster-agent")},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("datadog-cluster-agent"),
		},
		Subjects: rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      pulumi.String("datadog-cluster-agent"),
				Namespace: pulumi.String("default"),
			},
		},
	}, providerOpt, pulumi.DependsOn([]pulumi.Resource{sa, cr}))
	if err != nil {
		return err
	}

	_, err = rbacv1.NewClusterRoleBinding(ctx, "datadog-ca-auth-delegator", &rbacv1.ClusterRoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("datadog-cluster-agent-system-auth-delegator")},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("system:auth-delegator"),
		},
		Subjects: rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      pulumi.String("datadog-cluster-agent"),
				Namespace: pulumi.String("default"),
			},
		},
	}, providerOpt, pulumi.DependsOn([]pulumi.Resource{crb}))
	return err
}

func deployClusterAgent(ctx *pulumi.Context, clusterName, image string, authToken pulumi.StringOutput, providerOpt pulumi.ResourceOption) error {
	svc, err := corev1.NewService(ctx, "datadog-ca-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("datadog-cluster-agent"),
			Namespace: pulumi.String("default"),
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:     pulumi.String("ClusterIP"),
			Selector: pulumi.StringMap{"app": pulumi.String("datadog-cluster-agent")},
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:     pulumi.String("agentport"),
					Port:     pulumi.Int(5005),
					Protocol: pulumi.String("TCP"),
				},
			},
		},
	}, providerOpt)
	if err != nil {
		return err
	}

	_, err = appsv1.NewDeployment(ctx, "datadog-ca-deploy", &appsv1.DeploymentArgs{
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
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:            pulumi.String("cluster-agent"),
							Image:           pulumi.String(image),
							ImagePullPolicy: pulumi.String("IfNotPresent"),
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{Name: pulumi.String("agentport"), ContainerPort: pulumi.Int(5005), Protocol: pulumi.String("TCP")},
								&corev1.ContainerPortArgs{Name: pulumi.String("metricsapi"), ContainerPort: pulumi.Int(8443), Protocol: pulumi.String("TCP")},
							},
							Env: corev1.EnvVarArray{
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
								&corev1.EnvVarArgs{Name: pulumi.String("DD_LEADER_LEASE_DURATION"), Value: pulumi.String("15")},
								&corev1.EnvVarArgs{Name: pulumi.String("DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME"), Value: pulumi.String("datadog-cluster-agent")},
								&corev1.EnvVarArgs{Name: pulumi.String("DD_CLUSTER_AGENT_AUTH_TOKEN"), Value: authToken},
								&corev1.EnvVarArgs{Name: pulumi.String("DD_KUBE_RESOURCES_NAMESPACE"), Value: pulumi.String("default")},
								&corev1.EnvVarArgs{Name: pulumi.String("DD_COLLECT_KUBERNETES_EVENTS"), Value: pulumi.String("true")},
								&corev1.EnvVarArgs{Name: pulumi.String("DD_ORCHESTRATOR_EXPLORER_ENABLED"), Value: pulumi.String("true")},
								&corev1.EnvVarArgs{Name: pulumi.String("DD_ORCHESTRATOR_EXPLORER_CONTAINER_SCRUBBING_ENABLED"), Value: pulumi.String("true")},
							},
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
	}, providerOpt, pulumi.DependsOn([]pulumi.Resource{svc}))
	return err
}

func deployNodeAgentDaemonSet(ctx *pulumi.Context, clusterName, image string, authToken pulumi.StringOutput, fakeIntake *fakeintakeComp.Fakeintake, providerOpt pulumi.ResourceOption) error {
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
		env = append(env, &corev1.EnvVarArgs{
			Name:  pulumi.String("DD_DD_URL"),
			Value: fakeIntake.URL,
		})
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
								HttpGet:             &corev1.HTTPGetActionArgs{Path: pulumi.String("/health"), Port: pulumi.Int(5555)},
								InitialDelaySeconds: pulumi.Int(15),
								PeriodSeconds:       pulumi.Int(15),
								TimeoutSeconds:      pulumi.Int(5),
								SuccessThreshold:    pulumi.Int(1),
								FailureThreshold:    pulumi.Int(3),
							},
						},
					},
				},
			},
		},
	}, providerOpt)
	return err
}
