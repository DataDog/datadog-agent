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
	"encoding/json"
	"testing"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	provlocal "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	nginxNamespace  = "workload-nginx"
	nginxPort       = 80
	crName          = "nginx-http"
	httpCheckName   = "http_check"
	httpCheckMetric = "network.http.can_connect"
	nginxImageShort = "apps-nginx-server"
)

const helmValues = `
datadog:
  instrumentationCrd:
    enabled: true
  admissionController:
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

func agentOptions() []kubernetesagentparams.Option {
	return []kubernetesagentparams.Option{
		// TODO: Replace with remote helm when merged (https://github.com/DataDog/helm-charts/pull/2717)
		kubernetesagentparams.WithHelmRepoURL(""),
		kubernetesagentparams.WithHelmValues(helmValues),
	}
}

// k8sProvisioner picks between the AWS kindvm provisioner (default) and the
// local kind provisioner when E2E_DEV_LOCAL=true.
func k8sProvisioner() provisioners.TypedProvisioner[environments.Kubernetes] {
	if isLocalMode() {
		return provlocal.Provisioner(
			provlocal.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return nginx.K8sAppDefinition(e, kubeProvider, nginxNamespace, nginxPort, "", false, nil)
			}),
			provlocal.WithAgentOptions(agentOptions()...),
		)
	}
	return provkindvm.Provisioner(
		provkindvm.WithRunOptions(
			scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return nginx.K8sAppDefinition(e, kubeProvider, nginxNamespace, nginxPort, "", false, nil)
			}),
			scenkindvm.WithAgentOptions(agentOptions()...),
		),
	)
}

// isLocalMode returns true if E2E_DEV_LOCAL is set.
func isLocalMode() bool {
	devLocal, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.DevLocal, false)
	if err != nil {
		return false
	}
	return devLocal
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
// with a single http_check instance. tags, if non-nil, is the value of the
// per-instance `tags` list.
//
// Returned as *unstructured.Unstructured because the dynamic client requires
// it, but built from the typed datadoghqv1alpha1 struct so the call site is
// readable and field changes upstream cause a compile error rather than a
// silent runtime mismatch.
func newDDI(name string, tags []string) *unstructured.Unstructured {
	instance := map[string]any{
		"name": name,
		"url":  "http://%%host%%:80",
	}
	if tags != nil {
		instance["tags"] = tags
	}
	instanceRaw, err := json.Marshal(instance)
	if err != nil {
		panic(err)
	}

	cr := &datadoghqv1alpha1.DatadogInstrumentation{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "datadoghq.com/v1alpha1",
			Kind:       "DatadogInstrumentation",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: nginxNamespace,
		},
		Spec: datadoghqv1alpha1.DatadogInstrumentationSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "nginx",
			},
			Config: datadoghqv1alpha1.DatadogInstrumentationConfig{
				Checks: []datadoghqv1alpha1.DatadogInstrumentationCheckConfig{{
					Integration:    httpCheckName,
					ContainerImage: []string{nginxImageShort},
					InitConfig:     runtime.RawExtension{Raw: []byte(`{}`)},
					Instances:      []runtime.RawExtension{{Raw: instanceRaw}},
				}},
			},
		},
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cr)
	if err != nil {
		panic(err)
	}
	return &unstructured.Unstructured{Object: obj}
}

func (s *k8sTestSuite) applyDDI(ctx context.Context, ddi *unstructured.Unstructured) {
	_, err := s.dynClient.Resource(ddiGVR).Namespace(nginxNamespace).Create(ctx, ddi, metav1.CreateOptions{})
	require.NoErrorf(s.T(), err, "applying DatadogInstrumentation %s", ddi.GetName())
}

func (s *k8sTestSuite) deleteDDI(ctx context.Context, name string) {
	err := s.dynClient.Resource(ddiGVR).Namespace(nginxNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		require.NoErrorf(s.T(), err, "deleting DatadogInstrumentation %s", name)
	}
}

// TestCreateAndUpdate exercises the full CR -> DCA -> Node Agent -> check
// pipeline twice: first on initial create (the load-bearing assertion that
// the whole chain works end-to-end), then on a patch that adds a distinctive
// tag, proving the node agent re-fetched the config and re-scheduled the
// check rather than continuing to run the original.
func (s *k8sTestSuite) TestCreateAndUpdate() {
	t := s.T()
	ctx := context.Background()
	const updateTag = "ddi_e2e_phase:updated"

	s.applyDDI(ctx, newDDI(crName, nil))
	t.Cleanup(func() { s.deleteDDI(context.Background(), crName) })

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
	}, 2*time.Minute, 10*time.Second)

	// Update: patch the CR to attach a distinctive tag, then assert the metric
	// stream eventually carries it. Flushing fakeintake first means any tagged
	// sample we see came from a check run scheduled after the patch.
	patchObj := newDDI(crName, []string{updateTag})
	patch, err := json.Marshal(patchObj.Object)
	require.NoError(t, err)
	_, err = s.dynClient.Resource(ddiGVR).Namespace(nginxNamespace).Patch(ctx, crName, types.MergePatchType, patch, metav1.PatchOptions{})
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
		assert.NotEmptyf(c, metrics, "no %s metric with tag %s observed after CR update", httpCheckMetric, updateTag)
	}, 2*time.Minute, 10*time.Second)
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
		_, err := s.dynClient.Resource(ddiGVR).Namespace(nginxNamespace).Get(ctx, crName, metav1.GetOptions{})
		assert.NoError(c, err)
	}, 30*time.Second, 2*time.Second)

	dup := newDDI(crName+"-dup", nil)
	_, err := s.dynClient.Resource(ddiGVR).Namespace(nginxNamespace).Create(ctx, dup, metav1.CreateOptions{})
	require.Error(t, err, "duplicate-targetRef CR was admitted; validation webhook is not enforcing")
	t.Cleanup(func() { s.deleteDDI(context.Background(), crName+"-dup") })
}
