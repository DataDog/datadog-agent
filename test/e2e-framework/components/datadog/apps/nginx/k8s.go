package nginx

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/nginx/k8s"
	componentskube "github.com/DataDog/test-infra-definitions/components/kubernetes"

	"github.com/Masterminds/semver"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	autoscalingv2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/autoscaling/v2"
	autoscalingv2beta2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/autoscaling/v2beta2"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	policyv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/policy/v1"
	policyv1beta1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/policy/v1beta1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// K8sAppDefinition defines a Kubernetes application, with a deployment, a service, a pod disruption budget and an HPA.
// It also creates a DatadogMetric and an HPA if dependsOnCrd is not nil.
func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, namespace string, runtimeClass string, withDatadogAutoscaling bool, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	// The pulumi component resource names need to be unique. We adopt a naming convention of `namespace/componentName`.
	if err := e.Ctx().RegisterComponentResource("dd:apps", namespace+"/nginx", k8sComponent, opts...); err != nil {
		return nil, err
	}

	kubeVersion, err := semver.NewVersion(e.KubernetesVersion())
	if err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(k8sComponent))

	ns, err := corev1.NewNamespace(e.Ctx(), namespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"related_team": pulumi.String("contp"),
				"related_org":  pulumi.String("agent-org"),
			},
			Annotations: pulumi.StringMap{
				"related_email": pulumi.String("team-container-platform@datadoghq.com"),
			},
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	opts = append(opts, utils.PulumiDependsOn(ns))

	nginxManifest, err := k8s.NewNginxDeploymentManifest(namespace, k8s.WithRuntimeClass(runtimeClass))
	if err != nil {
		return nil, err
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), namespace+"/nginx", nginxManifest, opts...); err != nil {
		return nil, err
	}

	// In versions older than 1.21.0, we should use policyv1beta1
	kubeThresholdVersion, _ := semver.NewVersion("1.21.0")

	if kubeVersion.Compare(kubeThresholdVersion) < 0 {
		if _, err := policyv1beta1.NewPodDisruptionBudget(e.Ctx(), namespace+"/nginx", &policyv1beta1.PodDisruptionBudgetArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("nginx"),
				Namespace: pulumi.String(namespace),
				Labels: pulumi.StringMap{
					"app": pulumi.String("nginx"),
				},
			},
			Spec: &policyv1beta1.PodDisruptionBudgetSpecArgs{
				MaxUnavailable: pulumi.Int(1),
				Selector: &metav1.LabelSelectorArgs{
					MatchLabels: pulumi.StringMap{
						"app": pulumi.String("nginx"),
					},
				},
			},
		}, opts...); err != nil {
			return nil, err
		}
	} else {
		if _, err := policyv1.NewPodDisruptionBudget(e.Ctx(), namespace+"/nginx", &policyv1.PodDisruptionBudgetArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("nginx"),
				Namespace: pulumi.String(namespace),
				Labels: pulumi.StringMap{
					"app": pulumi.String("nginx"),
				},
			},
			Spec: &policyv1.PodDisruptionBudgetSpecArgs{
				MaxUnavailable: pulumi.Int(1),
				Selector: &metav1.LabelSelectorArgs{
					MatchLabels: pulumi.StringMap{
						"app": pulumi.String("nginx"),
					},
				},
			},
		}, opts...); err != nil {
			return nil, err
		}
	}

	if withDatadogAutoscaling {
		ddm, err := apiextensions.NewCustomResource(e.Ctx(), namespace+"/nginx", &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("datadoghq.com/v1alpha1"),
			Kind:       pulumi.String("DatadogMetric"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("nginx"),
				Namespace: pulumi.String(namespace),
				Labels: pulumi.StringMap{
					"app": pulumi.String("nginx"),
				},
			},
			OtherFields: map[string]interface{}{
				"spec": pulumi.Map{
					"query": pulumi.Sprintf("avg:nginx.net.request_per_s{kube_cluster_name:%%%%tag_kube_cluster_name%%%%,kube_namespace:%s,kube_deployment:nginx}.rollup(60)", namespace),
				},
			},
		}, opts...)
		if err != nil {
			return nil, err
		}

		// In versions older than 1.23.0, we should use autoscalingv2beta2
		kubeThresholdVersion, _ = semver.NewVersion("1.23.0")

		if kubeVersion.Compare(kubeThresholdVersion) < 0 {
			if _, err := autoscalingv2beta2.NewHorizontalPodAutoscaler(e.Ctx(), namespace+"/nginx", &autoscalingv2beta2.HorizontalPodAutoscalerArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Name:      pulumi.String("nginx"),
					Namespace: pulumi.String(namespace),
					Labels: pulumi.StringMap{
						"app": pulumi.String("nginx"),
					},
				},
				Spec: &autoscalingv2beta2.HorizontalPodAutoscalerSpecArgs{
					MinReplicas: pulumi.Int(1),
					MaxReplicas: pulumi.Int(5),
					ScaleTargetRef: &autoscalingv2beta2.CrossVersionObjectReferenceArgs{
						ApiVersion: pulumi.String("apps/v1"),
						Kind:       pulumi.String("Deployment"),
						Name:       pulumi.String("nginx"),
					},
					Metrics: &autoscalingv2beta2.MetricSpecArray{
						&autoscalingv2beta2.MetricSpecArgs{
							Type: pulumi.String("External"),
							External: &autoscalingv2beta2.ExternalMetricSourceArgs{
								Metric: &autoscalingv2beta2.MetricIdentifierArgs{
									Name: pulumi.String("datadogmetric@" + namespace + ":nginx"),
								},
								Target: &autoscalingv2beta2.MetricTargetArgs{
									Type:  pulumi.String("Value"),
									Value: pulumi.StringPtr("10"),
								},
							},
						},
					},
					Behavior: &autoscalingv2beta2.HorizontalPodAutoscalerBehaviorArgs{
						ScaleDown: &autoscalingv2beta2.HPAScalingRulesArgs{
							StabilizationWindowSeconds: pulumi.IntPtr(0),
						},
					},
				},
			}, append(opts, utils.PulumiDependsOn(ddm))...); err != nil {
				return nil, err
			}
		} else {
			if _, err := autoscalingv2.NewHorizontalPodAutoscaler(e.Ctx(), namespace+"/nginx", &autoscalingv2.HorizontalPodAutoscalerArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Name:      pulumi.String("nginx"),
					Namespace: pulumi.String(namespace),
					Labels: pulumi.StringMap{
						"app": pulumi.String("nginx"),
					},
				},
				Spec: &autoscalingv2.HorizontalPodAutoscalerSpecArgs{
					MinReplicas: pulumi.Int(1),
					MaxReplicas: pulumi.Int(5),
					ScaleTargetRef: &autoscalingv2.CrossVersionObjectReferenceArgs{
						ApiVersion: pulumi.String("apps/v1"),
						Kind:       pulumi.String("Deployment"),
						Name:       pulumi.String("nginx"),
					},
					Metrics: &autoscalingv2.MetricSpecArray{
						&autoscalingv2.MetricSpecArgs{
							Type: pulumi.String("External"),
							External: &autoscalingv2.ExternalMetricSourceArgs{
								Metric: &autoscalingv2.MetricIdentifierArgs{
									Name: pulumi.String("datadogmetric@" + namespace + ":nginx"),
								},
								Target: &autoscalingv2.MetricTargetArgs{
									Type:  pulumi.String("Value"),
									Value: pulumi.StringPtr("10"),
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehaviorArgs{
						ScaleDown: &autoscalingv2.HPAScalingRulesArgs{
							StabilizationWindowSeconds: pulumi.IntPtr(0),
						},
					},
				},
			}, append(opts, utils.PulumiDependsOn(ddm))...); err != nil {
				return nil, err
			}
		}

		if _, err := apiextensions.NewCustomResource(e.Ctx(), namespace+"/nginx", &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("autoscaling.k8s.io/v1"),
			Kind:       pulumi.String("VerticalPodAutoscaler"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("nginx"),
				Namespace: pulumi.String(namespace),
				Labels: pulumi.StringMap{
					"app": pulumi.String("nginx"),
				},
			},
			OtherFields: map[string]interface{}{
				"spec": pulumi.Map{
					"targetRef": pulumi.Map{
						"apiVersion": pulumi.String("apps/v1"),
						"kind":       pulumi.String("Deployment"),
						"name":       pulumi.String("nginx"),
					},
					"updatePolicy": pulumi.Map{
						"updateMode": pulumi.String("Auto"),
					},
				},
			},
		}, opts...); err != nil {
			return nil, err
		}
	}

	if _, err := corev1.NewService(e.Ctx(), namespace+"/nginx", k8s.NewNginxServiceManifest(namespace), opts...); err != nil {
		return nil, err
	}

	nginxQueryManifest, err := k8s.NewNginxQueryDeploymentManifest(namespace)
	if err != nil {
		return nil, err
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), namespace+"/nginx-query", nginxQueryManifest, opts...); err != nil {
		return nil, err
	}

	return k8sComponent, nil
}
