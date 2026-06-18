// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

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
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

const (
	nginxNamespace  = "ddi-workload-nginx"
	nginxPort       = 80
	crName          = "nginx-http"
	httpCheckName   = "http_check"
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

type ddiTestSuit struct {
	baseSuite[environments.Kubernetes]
	dynClient dynamic.Interface
}

func TestDDIAutodiscoverySuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ddiTestSuit{}, e2e.WithProvisioner(k8sProvisioner()))
}

func (s *ddiTestSuit) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.Fakeintake = s.Env().FakeIntake.Client()
	s.clusterName = s.Env().KubernetesCluster.ClusterName
	dyn, err := dynamic.NewForConfig(s.Env().KubernetesCluster.KubernetesClient.K8sConfig)
	require.NoError(s.T(), err)
	s.dynClient = dyn
}

// newDDI returns a DatadogInstrumentation CR targeting the nginx Deployment
// with a single http_check instance.
func newDDI(name string, tags []string) *unstructured.Unstructured {
	instance := map[string]any{
		"name": name,
		"url":  "http://%%host%%:%%port%%",
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

func (s *ddiTestSuit) applyDDI(ctx context.Context, ddi *unstructured.Unstructured) {
	_, err := s.dynClient.Resource(ddiGVR).Namespace(nginxNamespace).Create(ctx, ddi, metav1.CreateOptions{})
	require.NoErrorf(s.T(), err, "applying DatadogInstrumentation %s", ddi.GetName())
}

func (s *ddiTestSuit) deleteDDI(ctx context.Context, name string) {
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
func (s *ddiTestSuit) TestAutodiscoveryLifecycle() {
	t := s.T()
	ctx := context.Background()
	const updateTag = "ddi_e2e_phase:updated"

	// --- Create ---
	s.applyDDI(ctx, newDDI(crName, nil))

	s.testCheckRun(&testCheckRunArgs{
		Filter: testCheckRunFilterArgs{
			Name: "http.can_connect",
			Tags: []string{
				`^kube_namespace:` + nginxNamespace + `$`,
				`^kube_deployment:nginx$`,
			},
		},
		Expect: testCheckRunExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^kube_namespace:` + nginxNamespace + `$`,
				`^kube_deployment:nginx$`,
			},
			AcceptUnexpectedTags: true,
		},
	})

	// --- Update ---
	patchObj := newDDI(crName, []string{updateTag})
	patch, err := json.Marshal(patchObj.Object)
	require.NoError(t, err)
	_, err = s.dynClient.Resource(ddiGVR).Namespace(nginxNamespace).Patch(ctx, crName, types.MergePatchType, patch, metav1.PatchOptions{})
	require.NoError(t, err)
	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	s.testCheckRun(&testCheckRunArgs{
		Filter: testCheckRunFilterArgs{
			Name: "http.can_connect",
			Tags: []string{`^` + regexp.QuoteMeta(updateTag) + `$`},
		},
		Expect: testCheckRunExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^kube_namespace:` + nginxNamespace + `$`,
				`^kube_deployment:nginx$`,
			},
			AcceptUnexpectedTags: true,
		},
	})

	// --- Delete ---
	s.deleteDDI(ctx, crName)

	time.Sleep(20 * time.Second)

	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	require.Never(t, func() bool {
		runs, err := s.Env().FakeIntake.Client().FilterCheckRuns(
			"http.can_connect",
			fakeintake.WithMatchingTags[*aggregator.CheckRun]([]*regexp.Regexp{
				regexp.MustCompile(`^kube_namespace:` + nginxNamespace + `$`),
				regexp.MustCompile(`^kube_deployment:nginx$`),
			}),
		)
		if err != nil {
			return false
		}
		return len(runs) > 0
	}, 30*time.Second, 5*time.Second, "ddi check kept running after CR deletion")
}
