// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package custom

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func newServiceAccount(ns, name string, automount bool) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID(name),
			Name:      name,
			Namespace: ns,
		},
		AutomountServiceAccountToken: &automount,
	}
}

func newRoleBinding(ns, name string, subjects []rbacv1.Subject) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID(name),
			Name:      name,
			Namespace: ns,
		},
		Subjects: subjects,
	}
}

func newClusterRoleBinding(ns, name string, subjects []rbacv1.Subject) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID(name),
			Name:      name,
			Namespace: ns,
		},
		Subjects: subjects,
	}
}

func TestKubeDefaultServiceAccountsCheck(t *testing.T) {
	tests := []kubeApiserverFixture{
		{
			name:      "Default SA - No Roles",
			checkFunc: kubernetesDefaultServiceAccountsCheck,
			objects: []runtime.Object{
				newServiceAccount("ns1", "default", false),
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					compliance.KubeResourceFieldNamespace: "ns1",
					compliance.KubeResourceFieldName:      "default",
					compliance.KubeResourceFieldKind:      "ServiceAccount",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "",
				},
				Resource: compliance.ReportResource{
					ID:   "default",
					Type: "kube_serviceaccount",
				},
				Aggregated: true,
			},
		},
		{
			name:      "Default SA No automount",
			checkFunc: kubernetesDefaultServiceAccountsCheck,
			objects: []runtime.Object{
				newServiceAccount("ns1", "default", true),
				newRoleBinding("ns1", "rb1", []rbacv1.Subject{}),
			},
			expectReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
					compliance.KubeResourceFieldNamespace: "ns1",
					compliance.KubeResourceFieldName:      "default",
					compliance.KubeResourceFieldKind:      "ServiceAccount",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "",
				},
				Resource: compliance.ReportResource{
					ID:   "default",
					Type: "kube_serviceaccount",
				},
				Aggregated: true,
			},
		},
		{
			name:      "Default SA With RoleBinding",
			checkFunc: kubernetesDefaultServiceAccountsCheck,
			objects: []runtime.Object{
				newServiceAccount("ns1", "default", false),
				newRoleBinding("ns1", "rb1", []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						Namespace: "ns1",
						Name:      "default",
					},
				}),
			},
			expectReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
					compliance.KubeResourceFieldNamespace: "ns1",
					compliance.KubeResourceFieldName:      "default",
					compliance.KubeResourceFieldKind:      "ServiceAccount",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "",
				},
				Resource: compliance.ReportResource{
					ID:   "default",
					Type: "kube_serviceaccount",
				},
				Aggregated: true,
			},
		},
		{
			name:      "Default SA With ClusterRoleBinding",
			checkFunc: kubernetesDefaultServiceAccountsCheck,
			objects: []runtime.Object{
				newServiceAccount("ns1", "default", false),
				newClusterRoleBinding("", "crb1", []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						Namespace: "ns1",
						Name:      "default",
					},
				}),
			},
			expectReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
					compliance.KubeResourceFieldNamespace: "ns1",
					compliance.KubeResourceFieldName:      "default",
					compliance.KubeResourceFieldKind:      "ServiceAccount",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "",
				},
				Resource: compliance.ReportResource{
					ID:   "default",
					Type: "kube_serviceaccount",
				},
				Aggregated: true,
			},
		},
		{
			name:      "Default SA without any binding",
			checkFunc: kubernetesDefaultServiceAccountsCheck,
			objects: []runtime.Object{
				newServiceAccount("ns1", "default", false),
				newRoleBinding("ns2", "rb2", []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						Namespace: "ns2",
						Name:      "default",
					},
				}),
				newClusterRoleBinding("", "crb1", []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						Namespace: "ns1",
						Name:      "foo",
					},
				}),
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					compliance.KubeResourceFieldNamespace: "ns1",
					compliance.KubeResourceFieldName:      "default",
					compliance.KubeResourceFieldKind:      "ServiceAccount",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "",
				},
				Resource: compliance.ReportResource{
					ID:   "default",
					Type: "kube_serviceaccount",
				},
				Aggregated: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}
