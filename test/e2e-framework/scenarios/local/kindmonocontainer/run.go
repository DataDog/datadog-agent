// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kindmonocontainer provisions a local kind cluster and deploys the Datadog
// node agent (DaemonSet) and cluster agent (Deployment) as raw Kubernetes manifests
// without the Helm chart.
package kindmonocontainer

import (
	"fmt"

	kubernetesProvider "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
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

const clusterAgentLeaderElectionName = "datadog-leader-election"

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

		if err := deployNodeAgentRBAC(ctx, &localEnv, providerOpt); err != nil {
			return err
		}

		if err := deployClusterAgentRBAC(ctx, &localEnv, providerOpt); err != nil {
			return err
		}

		clusterAgentSvc, err := deployClusterAgent(ctx, clusterName, clusterAgentImage, clusterAgentToken.Result, fakeIntake, imgPullSecret, providerOpt)
		if err != nil {
			return err
		}

		if err := deployNodeAgentDaemonSet(ctx, clusterName, agentImage, clusterAgentToken.Result, fakeIntake, imgPullSecret, pulumi.DependsOn([]pulumi.Resource{clusterAgentSvc}), providerOpt); err != nil {
			return err
		}
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
					pulumi.String("limitranges"),
				},
				Verbs: pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("discovery.k8s.io")},
				Resources: pulumi.StringArray{pulumi.String("endpointslices")},
				Verbs:     pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
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
				ResourceNames: pulumi.StringArray{pulumi.String("datadogtoken"), pulumi.String(clusterAgentLeaderElectionName), pulumi.String("datadog-custom-metrics")},
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
			// Keep Lease RBAC in sync with DD_LEADER_LEASE_NAME in case this
			// scenario opts into Lease-based leader election.
			&rbacv1.PolicyRuleArgs{
				ApiGroups:     pulumi.StringArray{pulumi.String("coordination.k8s.io")},
				Resources:     pulumi.StringArray{pulumi.String("leases")},
				ResourceNames: pulumi.StringArray{pulumi.String(clusterAgentLeaderElectionName)},
				Verbs:         pulumi.StringArray{pulumi.String("get"), pulumi.String("update")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("coordination.k8s.io")},
				Resources: pulumi.StringArray{pulumi.String("leases")},
				Verbs:     pulumi.StringArray{pulumi.String("create")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{
					pulumi.String("persistentvolumes"),
					pulumi.String("persistentvolumeclaims"),
					pulumi.String("serviceaccounts"),
				},
				Verbs: pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("apps")},
				Resources: pulumi.StringArray{
					pulumi.String("statefulsets"),
				},
				Verbs: pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("networking.k8s.io")},
				Resources: pulumi.StringArray{
					pulumi.String("ingresses"),
					pulumi.String("networkpolicies"),
				},
				Verbs: pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("rbac.authorization.k8s.io")},
				Resources: pulumi.StringArray{
					pulumi.String("roles"),
					pulumi.String("rolebindings"),
					pulumi.String("clusterroles"),
					pulumi.String("clusterrolebindings"),
				},
				Verbs: pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("storage.k8s.io")},
				Resources: pulumi.StringArray{pulumi.String("storageclasses")},
				Verbs:     pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("apiextensions.k8s.io")},
				Resources: pulumi.StringArray{pulumi.String("customresourcedefinitions")},
				Verbs:     pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
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

func deployClusterAgent(ctx *pulumi.Context, clusterName, image string, authToken pulumi.StringOutput, fakeIntake *fakeintakeComp.Fakeintake, imgPullSecret *corev1.Secret, providerOpt pulumi.ResourceOption) (*corev1.Service, error) {
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
		return nil, err
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
	}, providerOpt, pulumi.DependsOn([]pulumi.Resource{svc}))
	return svc, err
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
