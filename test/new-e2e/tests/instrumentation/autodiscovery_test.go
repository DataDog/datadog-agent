// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !e2eunit

// Package instrumentation contains the end-to-end suite for DatadogInstrumentation
// CRD-backed Autodiscovery check scheduling.
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
`

// ddiGVR is the resource handle the dynamic client uses to CRUD CRs.
var ddiGVR = schema.GroupVersionResource{
	Group:    "datadoghq.com",
	Version:  "v1alpha1",
	Resource: "datadoginstrumentations",
}

// k8sProvisioner picks between the AWS kindvm provisioner (default) and the
// local kind provisioner when E2E_DEV_LOCAL=true.
func k8sProvisioner() provisioners.TypedProvisioner[environments.Kubernetes] {
	if isLocalMode() {
		return provlocal.Provisioner(
			provlocal.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return nginx.K8sAppDefinition(e, kubeProvider, nginxNamespace, nginxPort, "", false, nil)
			}),
			provlocal.WithAgentOptions(kubernetesagentparams.WithHelmValues(helmValues)),
		)
	}
	return provkindvm.Provisioner(
		provkindvm.WithRunOptions(
			scenkindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return nginx.K8sAppDefinition(e, kubeProvider, nginxNamespace, nginxPort, "", false, nil)
			}),
			scenkindvm.WithAgentOptions(kubernetesagentparams.WithHelmValues(helmValues)),
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
// with a single http_check instance.
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

// TestAutodiscoveryLifecycle exercises the full CR lifecycle: create -> update -> delete.
//
//  1. Create: assert the check runs and reports OK.
//  2. Update: patch the CR with a distinctive tag, assert it propagates.
//  3. Delete: remove the CR, assert the check stops running.
func (s *k8sTestSuite) TestAutodiscoveryLifecycle() {
	t := s.T()
	ctx := context.Background()
	const updateTag = "ddi_e2e_phase:updated"

	// --- Create ---
	s.applyDDI(ctx, newDDI(crName, nil))

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		runs, err := s.Env().FakeIntake.Client().FilterCheckRuns(httpCheckName)
		if !assert.NoError(c, err) {
			return
		}
		if !assert.NotEmpty(c, runs, "no http_check runs reported yet") {
			return
		}
		var sawOK bool
		for _, r := range runs {
			if r.Status == 0 {
				sawOK = true
				break
			}
		}
		assert.True(c, sawOK, "http_check ran but never reported OK")
	}, 2*time.Minute, 10*time.Second)

	// --- Update ---
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

	// --- Delete ---
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
