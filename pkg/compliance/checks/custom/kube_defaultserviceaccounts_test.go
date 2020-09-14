// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package custom

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func newServiceAccount(ns, name string, automount bool) *unstructured.Unstructured {
	sa := newUnstructured("v1", "ServiceAccount", ns, name, nil)
	sa.Object["automountServiceAccountToken"] = automount
	return sa
}

func newRoleBinding(ns, name, roletype string, subjects []rbacv1.Subject) *unstructured.Unstructured {
	rb := newUnstructured("rbac.authorization.k8s.io/v1", roletype, ns, name, nil)
	unstructuredSubjects := make([]interface{}, 0, len(subjects))
	for _, subject := range subjects {
		unstructuredSubject := map[string]interface{}{
			"kind":      subject.Kind,
			"namespace": subject.Namespace,
			"name":      subject.Name,
		}
		unstructuredSubjects = append(unstructuredSubjects, unstructuredSubject)
	}
	rb.Object["subjects"] = unstructuredSubjects
	return rb
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
			},
		},
		{
			name:      "Default SA No automount",
			checkFunc: kubernetesDefaultServiceAccountsCheck,
			objects: []runtime.Object{
				newServiceAccount("ns1", "default", true),
				newRoleBinding("ns1", "rb1", "RoleBinding", []rbacv1.Subject{}),
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
			},
		},
		{
			name:      "Default SA With RoleBinding",
			checkFunc: kubernetesDefaultServiceAccountsCheck,
			objects: []runtime.Object{
				newServiceAccount("ns1", "default", false),
				newRoleBinding("ns1", "rb1", "RoleBinding", []rbacv1.Subject{
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
			},
		},
		{
			name:      "Default SA With ClusterRoleBinding",
			checkFunc: kubernetesDefaultServiceAccountsCheck,
			objects: []runtime.Object{
				newServiceAccount("ns1", "default", false),
				newRoleBinding("", "crb1", "ClusterRoleBinding", []rbacv1.Subject{
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
			},
		},
		{
			name:      "Default SA without any binding",
			checkFunc: kubernetesDefaultServiceAccountsCheck,
			objects: []runtime.Object{
				newServiceAccount("ns1", "default", false),
				newRoleBinding("ns2", "rb2", "RoleBinding", []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						Namespace: "ns2",
						Name:      "default",
					},
				}),
				newRoleBinding("", "crb1", "ClusterRoleBinding", []rbacv1.Subject{
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}
