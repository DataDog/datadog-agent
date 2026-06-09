// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package instrumentation contains the end-to-end suite for DatadogInstrumentation
// CRD-backed Autodiscovery check scheduling (CONTP-1688). It exercises the full
// pipeline: a CR applied to the cluster -> DCA controller -> DCA endpoint
// (/api/v1/instrumentation/configs) -> Node Agent InstrumentationChecksConfigProvider ->
// the standard Autodiscovery pipeline -> check runs reported to fakeintake.
package instrumentation

import (
	"context"
	"testing"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	nginxNamespace = "workload-nginx"
	nginxPort      = 80
	// crNamespace must equal nginxNamespace — the AD handler builds its CEL
	// selector with container.pod.namespace == cr.Namespace, so the CR has to
	// live in the same namespace as the workload pods it targets.
	crNamespace     = nginxNamespace
	crName          = "nginx-http"
	httpCheckName   = "http_check"
	httpCheckMetric = "network.http.can_connect"
	nginxImageShort = "apps-nginx-server"
)

// helmValues turns on the DDI controller end-to-end:
//   - datadog.instrumentationCrd.enabled pulls the datadog-crds subchart
//     (installing the DatadogInstrumentation CRD), wires up the RBAC the
//     cluster agent needs to read DDIs, and sets
//     DD_INSTRUMENTATION_CRD_CONTROLLER_ENABLED=true on both the cluster
//     and node agent.
//   - admissionController.validation.enabled is required by CONTP-1649 so
//     the validation sub-test below actually has a webhook to reject CRs.
const helmValues = `
datadog:
  instrumentationCrd:
    enabled: true
  admissionController:
    enabled: true
    validation:
      enabled: true
`

// localHelmChartPath points at the in-progress helm-charts branch that adds
// `datadog.instrumentationCrd.enabled`. Once the chart change lands upstream,
// drop this constant and the WithHelmChartPath option below — the framework's
// default remote chart will already pull the right CRD subchart and RBAC.
const localHelmChartPath = "/Users/mathew.estafanous/go/src/github.com/DataDog/helm-charts/charts/datadog"

// ddiGVR is the resource handle the dynamic client uses to CRUD CRs.
var ddiGVR = schema.GroupVersionResource{
	Group:    "datadoghq.com",
	Version:  "v1alpha1",
	Resource: "datadoginstrumentations",
}

func k8sProvisioner() provisioners.TypedProvisioner[environments.Kubernetes] {
	return provkindvm.Provisioner(
		provkindvm.WithRunOptions(
			scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				// The DatadogInstrumentation CRD is installed by the chart itself
				// (via the datadog-crds subchart, gated by
				// datadog.instrumentationCrd.enabled). The DCA's CRD-wait at
				// startup (CONTP-1703) is satisfied by that, so the workload
				// callback only needs to deploy the target nginx workload.
				return nginx.K8sAppDefinition(e, kubeProvider, nginxNamespace, nginxPort, "", false, nil)
			}),
			scenkindvm.WithAgentOptions(
				kubernetesagentparams.WithHelmChartPath(localHelmChartPath),
				kubernetesagentparams.WithHelmValues(helmValues),
			),
		),
	)
}

type k8sTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	dynClient dynamic.Interface
}

func TestK8sTestSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &k8sTestSuite{}, e2e.WithProvisioner(k8sProvisioner()))
}

func (s *k8sTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	dyn, err := dynamic.NewForConfig(s.Env().KubernetesCluster.KubernetesClient.K8sConfig)
	require.NoError(s.T(), err)
	s.dynClient = dyn
}

// newDDI returns a DatadogInstrumentation CR targeting the nginx Deployment
// with a single http_check instance. tags is the value of the per-instance
// `tags` list; pass nil to omit.
func newDDI(name string, tags []any) *unstructured.Unstructured {
	instance := map[string]any{
		"name": name,
		"url":  "http://%%host%%:80",
	}
	if tags != nil {
		instance["tags"] = tags
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "datadoghq.com/v1alpha1",
		"kind":       "DatadogInstrumentation",
		"metadata": map[string]any{
			"name":      name,
			"namespace": crNamespace,
		},
		"spec": map[string]any{
			"targetRef": map[string]any{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "nginx",
			},
			"config": map[string]any{
				"checks": []any{
					map[string]any{
						"integration":    httpCheckName,
						"containerImage": []any{nginxImageShort},
						"initConfig":     map[string]any{},
						"instances":      []any{instance},
					},
				},
			},
		},
	}}
}

func (s *k8sTestSuite) applyDDI(ctx context.Context, ddi *unstructured.Unstructured) {
	_, err := s.dynClient.Resource(ddiGVR).Namespace(crNamespace).Create(ctx, ddi, metav1.CreateOptions{})
	require.NoError(s.T(), err, "applying DatadogInstrumentation %s", ddi.GetName())
}

func (s *k8sTestSuite) deleteDDI(ctx context.Context, name string) {
	err := s.dynClient.Resource(ddiGVR).Namespace(crNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		require.NoError(s.T(), err, "deleting DatadogInstrumentation %s", name)
	}
}

// TestCreate_HttpCheckRuns is the load-bearing assertion that the whole
// CR -> DCA -> Node Agent -> check pipeline works end-to-end. The other
// sub-tests only mutate state created here.
func (s *k8sTestSuite) TestCreate_HttpCheckRuns() {
	t := s.T()
	ctx := context.Background()

	s.applyDDI(ctx, newDDI(crName, nil))
	t.Cleanup(func() { s.deleteDDI(context.Background(), crName) })

	// 3 min budget: DCA poll (~30s) + node-agent provider poll (~30s) +
	// check.Interval (15s default) + intake aggregation lag. 10 s tick
	// keeps the loop inexpensive while still hitting each provider poll.
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		runs, err := s.Env().FakeIntake.Client().FilterCheckRuns(httpCheckName)
		if !assert.NoError(c, err) {
			return
		}
		if !assert.NotEmpty(c, runs, "no http_check runs reported yet") {
			return
		}
		// Status 0 == OK in the agent's check status enum.
		var sawOK bool
		for _, r := range runs {
			if r.Status == 0 {
				sawOK = true
				break
			}
		}
		assert.True(c, sawOK, "http_check ran but never reported OK")
	}, 3*time.Minute, 10*time.Second)
}

// TestUpdate_ReconfigureCheck patches the CR to add a distinctive tag, then
// asserts the metric stream eventually carries that tag — proving the node
// agent re-fetched the config and re-scheduled the check.
func (s *k8sTestSuite) TestUpdate_ReconfigureCheck() {
	t := s.T()
	ctx := context.Background()
	const updateTag = "ddi_e2e_phase:updated"

	s.applyDDI(ctx, newDDI(crName, nil))
	t.Cleanup(func() { s.deleteDDI(context.Background(), crName) })

	// Wait for at least one baseline run so the patch is observed as a change
	// rather than as the first appearance of the check.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		runs, err := s.Env().FakeIntake.Client().FilterCheckRuns(httpCheckName)
		assert.NoError(c, err)
		assert.NotEmpty(c, runs, "baseline http_check run not yet observed")
	}, 3*time.Minute, 10*time.Second)

	patch := []byte(`{"spec":{"config":{"checks":[{"integration":"http_check","containerImage":["apps-nginx-server"],"initConfig":{},"instances":[{"name":"nginx-http","url":"http://%%host%%:80","tags":["` + updateTag + `"]}]}]}}}`)
	_, err := s.dynClient.Resource(ddiGVR).Namespace(crNamespace).Patch(ctx, crName, types.MergePatchType, patch, metav1.PatchOptions{})
	require.NoError(t, err)

	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			httpCheckMetric,
			fakeintake.WithTags[*aggregator.MetricSeries]([]string{updateTag}),
		)
		if !assert.NoError(c, err) {
			return
		}
		assert.NotEmpty(c, metrics, "no %s metric with tag %s observed after CR update", httpCheckMetric, updateTag)
	}, 3*time.Minute, 10*time.Second)
}

// TestDelete_CheckStops asserts that removing the CR causes the node agent
// to unschedule the check — no new runs arrive in the post-delete window.
func (s *k8sTestSuite) TestDelete_CheckStops() {
	t := s.T()
	ctx := context.Background()

	s.applyDDI(ctx, newDDI(crName, nil))

	// Ensure the check ran at least once before we delete, otherwise the
	// "no runs after delete" assertion is meaningless.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		runs, err := s.Env().FakeIntake.Client().FilterCheckRuns(httpCheckName)
		assert.NoError(c, err)
		assert.NotEmpty(c, runs, "http_check did not run before delete")
	}, 3*time.Minute, 10*time.Second)

	s.deleteDDI(ctx, crName)
	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	// 90 s quiet window: longer than (DCA poll + node-agent poll + check
	// interval) so any still-scheduled check would have shown up at least once.
	assert.Never(t, func() bool {
		runs, err := s.Env().FakeIntake.Client().FilterCheckRuns(httpCheckName)
		if err != nil {
			return false
		}
		return len(runs) > 0
	}, 90*time.Second, 10*time.Second, "http_check kept running after CR delete")
}

// TestValidation_RejectsInvalid exercises the validation webhook (CONTP-1649)
// by applying a second CR with the same targetRef as an existing one. The
// duplicate-targetRef check should cause the API server to reject the call.
func (s *k8sTestSuite) TestValidation_RejectsInvalid() {
	t := s.T()
	ctx := context.Background()

	s.applyDDI(ctx, newDDI(crName, nil))
	t.Cleanup(func() { s.deleteDDI(context.Background(), crName) })

	// Wait briefly for the first CR to be visible via the lister the webhook
	// consults — otherwise the dup check has nothing to compare against.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := s.dynClient.Resource(ddiGVR).Namespace(crNamespace).Get(ctx, crName, metav1.GetOptions{})
		assert.NoError(c, err)
	}, 30*time.Second, 2*time.Second)

	dup := newDDI(crName+"-dup", nil)
	_, err := s.dynClient.Resource(ddiGVR).Namespace(crNamespace).Create(ctx, dup, metav1.CreateOptions{})
	require.Error(t, err, "duplicate-targetRef CR was admitted; validation webhook is not enforcing")
	t.Cleanup(func() { s.deleteDDI(context.Background(), crName+"-dup") })
}
