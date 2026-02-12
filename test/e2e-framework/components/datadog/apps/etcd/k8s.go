// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package etcd

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

// This stores an openmetrics check configuration in etcd.
// It runs the check against the example prometheus app already deployed
// ("apps-prometheus").
// The check renames one of the prometheus metrics so that we can verify that
// the check was discovered.
const storeCheckConfigScript = `
set -e

echo "[init] Waiting for etcd to respond on TCP port 2379..."
until nc -z localhost 2379; do
  sleep 1
done

echo "[init] Waiting for etcd v2 API to be ready..."
until curl -sf http://localhost:2379/v2/keys/; do
  echo "[init] etcd not ready yet..."
  sleep 1
done

echo "[init] Setting check configuration keys in etcd v2..."

curl -sf -XPUT http://localhost:2379/v2/keys/datadog/check_configs/apps-prometheus/check_names \
  --data-urlencode 'value=["openmetrics"]'

curl -sf -XPUT http://localhost:2379/v2/keys/datadog/check_configs/apps-prometheus/init_configs \
  --data-urlencode 'value=[{}]'

curl -sf -XPUT http://localhost:2379/v2/keys/datadog/check_configs/apps-prometheus/instances \
  --data-urlencode 'value=[{"openmetrics_endpoint": "http://%%host%%:8080/metrics", "metrics":[{"prom_gauge": "prom_gauge_configured_in_etcd"}]}]'

echo "[init] Done setting check configuration keys in etcd"
sleep infinity
`

const Namespace = "etcd"
const ServiceName = "etcd"

func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "etcd", k8sComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(k8sComponent))

	ns, err := corev1.NewNamespace(e.Ctx(), Namespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(Namespace),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	opts = append(opts, utils.PulumiDependsOn(ns))

	// openshift requires a non-default service account tighted to the privileged scc
	sa, err := corev1.NewServiceAccount(e.Ctx(), "etcd-sa", &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.StringPtr("etcd-sa"),
			Namespace: pulumi.StringPtr(Namespace),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	// create a clusterRoleBinding to bind the new service account with the existing privileged scc
	if _, err := rbacv1.NewRoleBinding(e.Ctx(), "etcd-scc-binding", &rbacv1.RoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("etcd-scc-binding"),
			Namespace: pulumi.String(Namespace),
		},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("system:openshift:scc:privileged"),
		},
		Subjects: rbacv1.SubjectArray{
			rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      sa.Metadata.Name().Elem(),
				Namespace: pulumi.String(Namespace),
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	_, err = appsv1.NewDeployment(e.Ctx(), "etcd", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("etcd"),
			Namespace: pulumi.String(Namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("etcd"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("etcd"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("etcd"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: sa.Metadata.Name().Elem(),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name: pulumi.String("etcd"),
							// The agent only supports the v2 API, which is not
							// supported anymore in newer versions of etcd.
							Image: pulumi.String("quay.io/coreos/etcd:v3.5.1"),
							Command: pulumi.StringArray{
								pulumi.String("etcd"),
							},
							Args: pulumi.ToStringArray([]string{
								// The agent only supports the v2 API, that's why we use --enable-v2.
								"--enable-v2",
								"--name=etcd-0",
								"--data-dir=/var/lib/etcd",
								"--listen-client-urls=http://0.0.0.0:2379",
								"--advertise-client-urls=http://etcd:2379",
								"--listen-peer-urls=http://0.0.0.0:2380",
								"--initial-advertise-peer-urls=http://etcd:2380",
								"--initial-cluster=etcd-0=http://etcd:2380",
								"--initial-cluster-token=etcd-cluster-1",
								"--initial-cluster-state=new",
							}),
							Ports: &corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									Name:          pulumi.String("etcd"),
									ContainerPort: pulumi.Int(2379),
									Protocol:      pulumi.String("TCP"),
								},
							},
							ReadinessProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Path:   pulumi.String("/health"),
									Port:   pulumi.Int(2379),
									Scheme: pulumi.String("HTTP"),
								},
								InitialDelaySeconds: pulumi.Int(10),
								TimeoutSeconds:      pulumi.Int(5),
							},
							LivenessProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Path:   pulumi.String("/health"),
									Port:   pulumi.Int(2379),
									Scheme: pulumi.String("HTTP"),
								},
								InitialDelaySeconds: pulumi.Int(10),
								TimeoutSeconds:      pulumi.Int(5),
							},
							VolumeMounts: &corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("etcd-data"),
									MountPath: pulumi.String("/var/lib/etcd"),
								},
							},
						},
						&corev1.ContainerArgs{
							Name:  pulumi.String("etcd-config"),
							Image: pulumi.String("ghcr.io/datadog/apps-alpine:" + apps.Version),
							Command: pulumi.StringArray{
								pulumi.String("/bin/sh"),
								pulumi.String("-c"),
							},
							Args: pulumi.StringArray{
								pulumi.String(storeCheckConfigScript),
							},
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name:     pulumi.String("etcd-data"),
							EmptyDir: &corev1.EmptyDirVolumeSourceArgs{},
						},
					},
				},
			},
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	_, err = corev1.NewService(e.Ctx(), ServiceName, &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(ServiceName),
			Namespace: pulumi.String(Namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("etcd"),
			},
		},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{
				"app": pulumi.String("etcd"),
			},
			Ports: &corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("client"),
					Port:       pulumi.Int(2379),
					TargetPort: pulumi.Int(2379),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	return k8sComponent, nil
}
