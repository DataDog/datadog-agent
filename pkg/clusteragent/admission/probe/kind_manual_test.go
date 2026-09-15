// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package probe

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This is a manual, throwaway verification against a real kind cluster (not
// part of CI). Skipped unless RUN_KIND_MANUAL_TEST=1 is set, since it needs a
// real API server reachable via the current kubeconfig context, and RBAC
// objects it creates/deletes as it goes to drive each scenario.
func TestManual_AgainstRealKindCluster(t *testing.T) {
	if os.Getenv("RUN_KIND_MANUAL_TEST") != "1" {
		t.Skip("set RUN_KIND_MANUAL_TEST=1 to run against the current kubeconfig context's cluster")
	}

	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).ClientConfig()
	require.NoError(t, err)

	adminClient, err := kubernetes.NewForConfig(restCfg)
	require.NoError(t, err)

	saUsername := "system:serviceaccount:test-probe-ns:datadog-cluster-agent"
	restrictedClient := impersonatedClient(t, restCfg, saUsername)

	ctx := context.Background()

	// webhookExists calls admcommon's package-level dispatch vars, which
	// default to a stub returning "admission controller not started" until
	// something wires them to the real v1 implementations (normally done at
	// cluster agent startup in start.go). Point them at the real
	// implementations here so the probe actually queries the API server.
	prevValidating := admcommon.GetValidatingWebhookStatus
	prevMutating := admcommon.GetMutatingWebhookStatus
	admcommon.GetValidatingWebhookStatus = admcommon.GetValidatingWebhookStatusV1
	admcommon.GetMutatingWebhookStatus = admcommon.GetMutatingWebhookStatusV1
	t.Cleanup(func() {
		admcommon.GetValidatingWebhookStatus = prevValidating
		admcommon.GetMutatingWebhookStatus = prevMutating
	})

	newProbe := func(client kubernetes.Interface) (*Probe, *healthplatformmock.Mock) {
		hp := healthplatformmock.New(t)
		return &Probe{
			k8sClient:      client,
			namespace:      "test-probe-ns",
			webhookName:    "datadog-webhook",
			isLeaderFunc:   func() bool { return true },
			logLimiter:     log.NewLogLimit(10, time.Minute),
			healthPlatform: hp,
			diagnosticHint: "check network",
		}, hp
	}

	revokeAllRBAC(ctx, t, adminClient)

	t.Run("1_CauseProbeConfigForbidden_no_rbac_at_all", func(t *testing.T) {
		p, hp := newProbe(restrictedClient)
		p.mutationEnabled = true
		p.runProbe(ctx)

		issue := hp.GetIssue(healthIssueID)
		require.NotNil(t, issue, "expected a health issue to be reported")
		t.Logf("Description: %s", issue.Description)
		for _, s := range issue.Remediation.Steps {
			t.Logf("Step %d: %s", s.Order, s.Text)
		}
		assert.Contains(t, issue.Description, "does not have permission to create configmaps")
		assert.Contains(t, issue.Description, saUsername, "real API server message should have been parsed for the exact identity")
		require.NotEmpty(t, issue.Remediation.Steps)
		assert.Contains(t, issue.Remediation.Steps[0].Text, saUsername)
		assert.Contains(t, issue.Remediation.Steps[0].Text, `"create"`)
		assert.Contains(t, issue.Remediation.Steps[0].Text, "configmaps")
	})

	grantConfigMapCreate(ctx, t, adminClient)
	grantWebhookConfigGet(ctx, t, adminClient)

	t.Run("2_CauseWebhookMissing_confirmed_absent", func(t *testing.T) {
		p, hp := newProbe(restrictedClient)
		p.mutationEnabled = true
		p.runProbe(ctx)

		issue := hp.GetIssue(healthIssueID)
		require.NotNil(t, issue)
		t.Logf("Description: %s", issue.Description)
		for _, s := range issue.Remediation.Steps {
			t.Logf("Step %d: %s", s.Order, s.Text)
		}
		assert.Contains(t, issue.Description, `"datadog-webhook" does not exist`)
		assert.Contains(t, issue.Description, "deleted after being created")
		for _, s := range issue.Remediation.Steps {
			assert.NotContains(t, s.Text, "kubectl get")
		}
	})

	revokeWebhookConfigGet(ctx, t, adminClient)

	t.Run("3_CauseIndeterminate_rbac_denies_lookup", func(t *testing.T) {
		p, hp := newProbe(restrictedClient)
		p.mutationEnabled = true
		p.runProbe(ctx)

		issue := hp.GetIssue(healthIssueID)
		require.NotNil(t, issue)
		t.Logf("Description: %s", issue.Description)
		for _, s := range issue.Remediation.Steps {
			t.Logf("Step %d: %s", s.Order, s.Text)
		}
		require.NotEmpty(t, issue.Remediation.Steps)
		assert.Contains(t, issue.Remediation.Steps[0].Text, saUsername)
		assert.Contains(t, issue.Remediation.Steps[0].Text, `"get"`)
		assert.Contains(t, issue.Remediation.Steps[0].Text, "mutatingwebhookconfigurations")
	})

	grantWebhookConfigGet(ctx, t, adminClient)
	createWebhookConfig(ctx, t, adminClient)

	t.Run("4_default_network_cause_webhook_confirmed_exists", func(t *testing.T) {
		p, hp := newProbe(restrictedClient)
		p.mutationEnabled = true
		p.runProbe(ctx)

		issue := hp.GetIssue(healthIssueID)
		require.NotNil(t, issue)
		t.Logf("Description: %s", issue.Description)
		for _, s := range issue.Remediation.Steps {
			t.Logf("Step %d: %s", s.Order, s.Text)
		}
		assert.NotContains(t, issue.Description, "does not exist")
	})

	t.Run("5_secret_controller_forbidden_real_error_shape", func(t *testing.T) {
		_, rawErr := restrictedClient.CoreV1().Secrets("test-probe-ns").Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "datadog-webhook-certificate"},
		}, metav1.CreateOptions{})
		require.Error(t, rawErr, "secrets create was never granted to the SA")
		t.Logf("raw API server error: %v", rawErr)

		// The real secret controller wraps its create/update errors with
		// WrapIfForbidden before storing them as LastReconcileError (see
		// controllers/secret/controller.go) so rbacFixHint can classify them;
		// mirror that here rather than handing the probe a raw client-go error.
		wrappedErr := admcommon.WrapIfForbidden(rawErr, "create", "secrets", "test-probe-ns", "datadog-webhook-certificate")

		p, hp := newProbe(restrictedClient)
		p.mutationEnabled = true
		p.secretController = &fakeReconcileStatusProvider{err: wrappedErr}
		p.runProbe(ctx)

		issue := hp.GetIssue(healthIssueID)
		require.NotNil(t, issue)
		t.Logf("Description: %s", issue.Description)
		for _, s := range issue.Remediation.Steps {
			t.Logf("Step %d: %s", s.Order, s.Text)
		}
		require.NotEmpty(t, issue.Remediation.Steps)
		assert.Contains(t, issue.Remediation.Steps[0].Text, saUsername)
		assert.Contains(t, issue.Remediation.Steps[0].Text, `"create"`)
		assert.Contains(t, issue.Remediation.Steps[0].Text, "secrets")
	})
}

func impersonatedClient(t *testing.T, base *rest.Config, username string) kubernetes.Interface {
	cfg := rest.CopyConfig(base)
	cfg.Impersonate = rest.ImpersonationConfig{
		UserName: username,
		Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:test-probe-ns", "system:authenticated"},
	}
	client, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err)
	return client
}

func revokeAllRBAC(ctx context.Context, t *testing.T, client kubernetes.Interface) {
	_ = client.RbacV1().RoleBindings("test-probe-ns").Delete(ctx, "datadog-cluster-agent-probe", metav1.DeleteOptions{})
	_ = client.RbacV1().Roles("test-probe-ns").Delete(ctx, "datadog-cluster-agent-probe", metav1.DeleteOptions{})
	_ = client.RbacV1().ClusterRoleBindings().Delete(ctx, "datadog-cluster-agent-probe-webhooks", metav1.DeleteOptions{})
	_ = client.RbacV1().ClusterRoles().Delete(ctx, "datadog-cluster-agent-probe-webhooks", metav1.DeleteOptions{})
	_ = client.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(ctx, "datadog-webhook", metav1.DeleteOptions{})
	waitForRBACPropagation(t)
}

func grantConfigMapCreate(ctx context.Context, t *testing.T, client kubernetes.Interface) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-cluster-agent-probe", Namespace: "test-probe-ns"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"create"}},
		},
	}
	_, err := client.RbacV1().Roles("test-probe-ns").Create(ctx, role, metav1.CreateOptions{})
	require.NoError(t, err)

	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-cluster-agent-probe", Namespace: "test-probe-ns"},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "datadog-cluster-agent", Namespace: "test-probe-ns"},
		},
		RoleRef: rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "datadog-cluster-agent-probe"},
	}
	_, err = client.RbacV1().RoleBindings("test-probe-ns").Create(ctx, binding, metav1.CreateOptions{})
	require.NoError(t, err)

	waitForRBACPropagation(t)
}

func createWebhookConfig(ctx context.Context, t *testing.T, client kubernetes.Interface) {
	sideEffects := admissionregistrationv1.SideEffectClassNone
	url := "https://127.0.0.1:8000/dummy"
	_, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(ctx, &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-webhook"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "webhook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					URL: &url,
				},
				SideEffects:             &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
}

func grantWebhookConfigGet(ctx context.Context, t *testing.T, client kubernetes.Interface) {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-cluster-agent-probe-webhooks"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"admissionregistration.k8s.io"}, Resources: []string{"mutatingwebhookconfigurations", "validatingwebhookconfigurations"}, Verbs: []string{"get"}},
		},
	}
	_, err := client.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
	require.NoError(t, err)

	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-cluster-agent-probe-webhooks"},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "datadog-cluster-agent", Namespace: "test-probe-ns"},
		},
		RoleRef: rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "datadog-cluster-agent-probe-webhooks"},
	}
	_, err = client.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
	require.NoError(t, err)

	waitForRBACPropagation(t)
}

func revokeWebhookConfigGet(ctx context.Context, t *testing.T, client kubernetes.Interface) {
	require.NoError(t, client.RbacV1().ClusterRoleBindings().Delete(ctx, "datadog-cluster-agent-probe-webhooks", metav1.DeleteOptions{}))
	require.NoError(t, client.RbacV1().ClusterRoles().Delete(ctx, "datadog-cluster-agent-probe-webhooks", metav1.DeleteOptions{}))
	waitForRBACPropagation(t)
}

func waitForRBACPropagation(t *testing.T) {
	t.Helper()
	time.Sleep(2 * time.Second)
}
