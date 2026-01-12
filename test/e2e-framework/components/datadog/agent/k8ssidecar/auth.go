// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package k8ssidecar

import (
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	datadogSecretName      = "datadog-secret"
	serviceAccountName     = "datadog-service-account"
	clusterRoleName        = "datadog-cluster-role"
	clusterRoleBindingName = "datadog-cluster-role-binding"
)

// NewServiceAccount creates a ServiceAccount
func NewServiceAccount(ctx *pulumi.Context, namespace string, name string, opts ...pulumi.ResourceOption) (*corev1.ServiceAccount, error) {
	pulumiName := namespace + "-" + name
	return corev1.NewServiceAccount(ctx, pulumiName, &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(namespace),
		},
	}, opts...)
}

// NewDatadogSecret creates a Secret named datadog-secret with two fields: 1) api-key 2)token
func NewDatadogSecret(ctx *pulumi.Context, namespace string, apiKey pulumi.StringInput,
	token pulumi.StringInput, opts ...pulumi.ResourceOption) (*corev1.Secret, error) {
	pulumiName := namespace + "-" + datadogSecretName
	return corev1.NewSecret(ctx,
		pulumiName,
		&corev1.SecretArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(datadogSecretName),
				Namespace: pulumi.String(namespace),
			},
			StringData: pulumi.StringMap{
				"api-key": apiKey,
				"token":   token,
			},
		}, opts...)
}

// NewAgentClusterRole creates a cluster role for a sidecar agent
func NewAgentClusterRole(ctx *pulumi.Context, name string, opts ...pulumi.ResourceOption) (*rbacv1.ClusterRole, error) {
	return rbacv1.NewClusterRole(ctx, name, &rbacv1.ClusterRoleArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(name),
		},
		Rules: rbacv1.PolicyRuleArray{
			rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{
					pulumi.String(""),
				},
				Resources: pulumi.StringArray{
					pulumi.String("nodes"),
					pulumi.String("namespaces"),
					pulumi.String("endpoints"),
				},
				Verbs: pulumi.StringArray{
					pulumi.String("get"),
					pulumi.String("list"),
				},
			},
			rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{
					pulumi.String(""),
				},
				Resources: pulumi.StringArray{
					pulumi.String("nodes/metrics"),
					pulumi.String("nodes/metrics"),
					pulumi.String("nodes/spec"),
					pulumi.String("nodes/stats"),
					pulumi.String("nodes/proxy"),
					pulumi.String("nodes/pods"),
					pulumi.String("nodes/healthz"),
				},
				Verbs: pulumi.StringArray{
					pulumi.String("get"),
				},
			},
		},
	}, opts...)
}

// NewClusterRoleBinding creates a cluster role binding
func NewClusterRoleBinding(ctx *pulumi.Context, name string, clusterRole *rbacv1.ClusterRole,
	serviceAccount *corev1.ServiceAccount, opts ...pulumi.ResourceOption) (*rbacv1.ClusterRoleBinding, error) {
	return rbacv1.NewClusterRoleBinding(ctx, name, &rbacv1.ClusterRoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(name),
		},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     clusterRole.Kind,
			Name:     clusterRole.Metadata.Name().Elem(),
		},
		Subjects: rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind:      serviceAccount.Kind,
				Name:      serviceAccount.Metadata.Name().Elem(),
				Namespace: serviceAccount.Metadata.Namespace().Elem(),
			},
		},
	}, opts...)
}

// NewServiceAccountWithClusterPermissions creates a cluster role with default permissions, and returns a service account
// with those permissions attached
func NewServiceAccountWithClusterPermissions(ctx *pulumi.Context, namespace string, apiKey pulumi.StringInput,
	clusterAgentToken pulumi.StringInput, opts ...pulumi.ResourceOption) (*corev1.ServiceAccount, error) {

	_, err := NewDatadogSecret(ctx, namespace, apiKey, clusterAgentToken, opts...)
	if err != nil {
		return nil, err
	}

	serviceAccount, err := NewServiceAccount(ctx, namespace, serviceAccountName, opts...)
	if err != nil {
		return nil, err
	}

	// Not namespaced so name must be globally unique
	uniqueClusterRoleName := namespace + "-" + clusterRoleName
	clusterRole, err := NewAgentClusterRole(ctx, uniqueClusterRoleName, opts...)
	if err != nil {
		return nil, err
	}

	// Not namespaced so name must be globally unique
	uniqueClusterRoleBindingName := namespace + "-" + clusterRoleBindingName
	_, err = NewClusterRoleBinding(ctx, uniqueClusterRoleBindingName, clusterRole, serviceAccount, opts...)
	if err != nil {
		return nil, err
	}

	return serviceAccount, nil
}
