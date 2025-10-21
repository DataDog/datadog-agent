package cpustress

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps"
	componentskube "github.com/DataDog/test-infra-definitions/components/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func K8sAppDefinitionOpenShift(e config.Env, kubeProvider *kubernetes.Provider, namespace string, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", fmt.Sprintf("%s-cpustress-openshift", namespace), k8sComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(k8sComponent))

	ns, err := corev1.NewNamespace(e.Ctx(), namespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(namespace),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	opts = append(opts, utils.PulumiDependsOn(ns))

	if _, err := appsv1.NewDeployment(e.Ctx(), fmt.Sprintf("%s/stress-ng", namespace), &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("stress-ng"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("stress-ng"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("stress-ng"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("stress-ng"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name:  pulumi.String("stress-ng"),
							Image: pulumi.String("ghcr.io/datadog/apps-stress-ng:" + apps.Version),
							Args: pulumi.StringArray{
								pulumi.String("--cpu=1"),
								pulumi.String("--cpu-load=15"),
							},
							WorkingDir: pulumi.String("/tmp"),
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("200m"),
									"memory": pulumi.String("128Mi"),
								},
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("100m"),
									"memory": pulumi.String("64Mi"),
								},
							},
							VolumeMounts: corev1.VolumeMountArray{
								corev1.VolumeMountArgs{
									Name:      pulumi.String("temp-dir"),
									MountPath: pulumi.String("/tmp"),
								},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						corev1.VolumeArgs{
							Name:     pulumi.String("temp-dir"),
							EmptyDir: &corev1.EmptyDirVolumeSourceArgs{},
						},
					},
				},
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	return k8sComponent, nil
}
