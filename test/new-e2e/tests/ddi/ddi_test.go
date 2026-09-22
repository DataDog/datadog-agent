// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ddi contains end-to-end tests for DatadogInstrumentation.
package ddi

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	k8syaml "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provisionerkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	testNamespace       = "ddi-e2e"
	updateTestNamespace = "ddi-e2e-update"
	workloadName        = "nginx"
	containerName       = "nginx"
	ddiName             = "nginx-monitoring"
	updateDDIName       = "nginx-monitoring-update"
	metricName          = "network.http.response_time"
	logService          = "ddi-e2e-nginx"
	updateLogService    = "ddi-e2e-nginx-update"
	logSource           = "nginx"
	logMessage          = "GET / HTTP/1.1"
	testTag             = "ddi_e2e:true"
	timeout             = 1 * time.Minute
	interval            = 1 * time.Second
)

var ddiGVR = schema.GroupVersionResource{
	Group:    "datadoghq.com",
	Version:  "v1alpha1",
	Resource: "datadoginstrumentations",
}

//go:embed fixtures/helm-values.yaml
var helmValues string

type ddiSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestDDI(t *testing.T) {
	t.Parallel()

	e2e.Run(t, &ddiSuite{},
		e2e.WithProvisioner(provisionerkindvm.Provisioner(
			provisionerkindvm.WithRunOptions(
				scenariokindvm.WithAgentOptions(
					kubernetesagentparams.WithoutLogsContainerCollectAll(),
					kubernetesagentparams.WithHelmValues(helmValues),
				),
				scenariokindvm.WithAgentDependentWorkloadApp(ddiWorkload),
			),
		)),
	)
}

func (s *ddiSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
}

func (s *ddiSuite) TestCheckMetricCollection() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			metricName,
			fakeintake.WithTags[*aggregator.MetricSeries]([]string{testTag, "kube_namespace:" + testNamespace}),
		)
		require.NoError(c, err)
		if len(metrics) == 0 {
			metricNames, namesErr := s.Env().FakeIntake.Client().GetMetricNames()
			assert.NoError(c, namesErr)
			assert.Fail(c, "DDI-configured check metric not found", "available metrics: %v", metricNames)
		}
	}, timeout, interval)
}

func (s *ddiSuite) TestLogCollection() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(
			logService,
			fakeintake.WithMessageContaining(logMessage),
			fakeintake.WithTags[*aggregator.Log]([]string{testTag, "kube_namespace:" + testNamespace}),
		)
		require.NoError(c, err)
		if len(logs) == 0 {
			services, servicesErr := s.Env().FakeIntake.Client().GetLogServiceNames()
			assert.NoError(c, servicesErr)
			assert.Fail(c, "DDI-configured log not found", "available log services: %v", services)
			return
		}
		assert.Equal(c, logSource, logs[0].Source)
	}, timeout, interval)
}

func (s *ddiSuite) TestReadyConditions() {
	dynamicClient, err := dynamic.NewForConfig(s.Env().KubernetesCluster.KubernetesClient.K8sConfig)
	require.NoError(s.T(), err)

	s.EventuallyWithT(func(c *assert.CollectT) {
		resource, err := dynamicClient.Resource(ddiGVR).Namespace(testNamespace).Get(
			context.Background(),
			ddiName,
			metav1.GetOptions{},
		)
		require.NoError(c, err)

		conditions, found, err := unstructured.NestedSlice(resource.Object, "status", "conditions")
		require.NoError(c, err)
		if !assert.True(c, found, "DatadogInstrumentation has no status conditions") {
			return
		}
		assert.True(c, hasTrueCondition(conditions, "ChecksReady"), "ChecksReady is not True: %v", conditions)
		assert.True(c, hasTrueCondition(conditions, "LogsReady"), "LogsReady is not True: %v", conditions)
	}, timeout, interval)
}

func (s *ddiSuite) TestConfigurationUpdate() {
	dynamicClient, err := dynamic.NewForConfig(s.Env().KubernetesCluster.KubernetesClient.K8sConfig)
	require.NoError(s.T(), err)

	ctx := context.Background()
	resourceClient := dynamicClient.Resource(ddiGVR).Namespace(updateTestNamespace)
	resource, err := resourceClient.Get(ctx, updateDDIName, metav1.GetOptions{})
	require.NoError(s.T(), err)
	previousGeneration := resource.GetGeneration()
	updatedTestTag := fmt.Sprintf("ddi_e2e_update:%d", time.Now().UnixNano())

	ddi := &datadoghq.DatadogInstrumentation{}
	require.NoError(s.T(), runtime.DefaultUnstructuredConverter.FromUnstructured(resource.Object, ddi))
	require.Len(s.T(), ddi.Spec.Config.Checks, 1)
	require.Len(s.T(), ddi.Spec.Config.Checks[0].Instances, 1)
	require.Len(s.T(), ddi.Spec.Config.Logs, 1)

	updatedInstance, err := newHTTPCheckInstance([]string{testTag, updatedTestTag})
	require.NoError(s.T(), err)
	ddi.Spec.Config.Checks[0].Instances[0] = updatedInstance
	ddi.Spec.Config.Logs[0].Tags = []string{testTag, updatedTestTag}

	updatedObject, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ddi)
	require.NoError(s.T(), err)
	updatedResource, err := resourceClient.Update(ctx, &unstructured.Unstructured{Object: updatedObject}, metav1.UpdateOptions{})
	require.NoError(s.T(), err)
	updatedGeneration := updatedResource.GetGeneration()
	require.Greater(s.T(), updatedGeneration, previousGeneration)

	s.EventuallyWithT(func(c *assert.CollectT) {
		resource, err := resourceClient.Get(ctx, updateDDIName, metav1.GetOptions{})
		require.NoError(c, err)

		conditions, found, err := unstructured.NestedSlice(resource.Object, "status", "conditions")
		require.NoError(c, err)
		if !assert.True(c, found, "updated DatadogInstrumentation has no status conditions") {
			return
		}
		assert.True(c, hasTrueConditionForGeneration(conditions, "ChecksReady", updatedGeneration), "ChecksReady is not True for generation %d: %v", updatedGeneration, conditions)
		assert.True(c, hasTrueConditionForGeneration(conditions, "LogsReady", updatedGeneration), "LogsReady is not True for generation %d: %v", updatedGeneration, conditions)
	}, timeout, interval)

	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			metricName,
			fakeintake.WithTags[*aggregator.MetricSeries]([]string{testTag, updatedTestTag, "kube_namespace:" + updateTestNamespace}),
		)
		require.NoError(c, err)
		if len(metrics) == 0 {
			metricNames, namesErr := s.Env().FakeIntake.Client().GetMetricNames()
			assert.NoError(c, namesErr)
			assert.Fail(c, "updated DDI-configured check metric not found", "available metrics: %v", metricNames)
		}

		logs, err := s.Env().FakeIntake.Client().FilterLogs(
			updateLogService,
			fakeintake.WithMessageContaining(logMessage),
			fakeintake.WithTags[*aggregator.Log]([]string{testTag, updatedTestTag, "kube_namespace:" + updateTestNamespace}),
		)
		require.NoError(c, err)
		if len(logs) == 0 {
			services, servicesErr := s.Env().FakeIntake.Client().GetLogServiceNames()
			assert.NoError(c, servicesErr)
			assert.Fail(c, "updated DDI-configured log not found", "available log services: %v", services)
			return
		}
		assert.Equal(c, logSource, logs[0].Source)
	}, timeout, interval)
}

func hasTrueCondition(conditions []any, conditionType string) bool {
	for _, rawCondition := range conditions {
		condition, ok := rawCondition.(map[string]any)
		if !ok {
			continue
		}
		if condition["type"] == conditionType && condition["status"] == "True" {
			return true
		}
	}
	return false
}

func hasTrueConditionForGeneration(conditions []any, conditionType string, generation int64) bool {
	for _, rawCondition := range conditions {
		condition, ok := rawCondition.(map[string]any)
		if !ok || condition["type"] != conditionType || condition["status"] != "True" {
			continue
		}
		observedGeneration, found, err := unstructured.NestedInt64(condition, "observedGeneration")
		if err == nil && found && observedGeneration == generation {
			return true
		}
	}
	return false
}

func ddiWorkload(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*kubecomp.Workload, error) {
	workload, err := newDDIWorkload(e, kubeProvider, testNamespace, ddiName, logService, dependsOnAgent)
	if err != nil {
		return nil, err
	}
	if _, err := newDDIWorkload(e, kubeProvider, updateTestNamespace, updateDDIName, updateLogService, dependsOnAgent); err != nil {
		return nil, err
	}
	return workload, nil
}

func newDDIWorkload(e config.Env, kubeProvider *kubernetes.Provider, namespace, instrumentationName, service string, dependsOnAgent pulumi.ResourceOption) (*kubecomp.Workload, error) {
	workload, err := nginx.K8sAppDefinitionWithOptions(
		e,
		kubeProvider,
		namespace,
		80,
		"",
		false,
		[]nginx.K8sAppOption{nginx.WithoutDatadogAnnotations()},
		dependsOnAgent,
	)
	if err != nil {
		return nil, err
	}

	ddiOptions := []pulumi.ResourceOption{
		pulumi.Provider(kubeProvider),
		pulumi.Parent(workload),
		pulumi.DeletedWith(kubeProvider),
		dependsOnAgent,
		utils.PulumiDependsOn(workload),
	}
	ddi, err := newDatadogInstrumentation(namespace, instrumentationName, service)
	if err != nil {
		return nil, err
	}
	ddiManifest, err := json.Marshal(ddi)
	if err != nil {
		return nil, err
	}
	_, err = k8syaml.NewConfigGroup(e.Ctx(), namespace+"/"+instrumentationName, &k8syaml.ConfigGroupArgs{
		YAML: []string{string(ddiManifest)},
	}, ddiOptions...)
	if err != nil {
		return nil, err
	}

	return workload, nil
}

func newDatadogInstrumentation(namespace, instrumentationName, service string) (*datadoghq.DatadogInstrumentation, error) {
	initConfig, err := rawExtension(map[string]any{})
	if err != nil {
		return nil, err
	}
	instance, err := newHTTPCheckInstance([]string{testTag})
	if err != nil {
		return nil, err
	}

	return &datadoghq.DatadogInstrumentation{
		TypeMeta: metav1.TypeMeta{
			APIVersion: datadoghq.GroupVersion.String(),
			Kind:       "DatadogInstrumentation",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      instrumentationName,
			Namespace: namespace,
		},
		Spec: datadoghq.DatadogInstrumentationSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       workloadName,
			},
			Config: datadoghq.DatadogInstrumentationConfig{
				Checks: []datadoghq.DatadogInstrumentationCheckConfig{
					{
						Integration:   "http_check",
						ContainerName: containerName,
						InitConfig:    initConfig,
						Instances:     []runtime.RawExtension{instance},
					},
				},
				Logs: []datadoghq.DatadogInstrumentationLogConfig{
					{
						ContainerName: containerName,
						DatadogInstrumentationLogFields: datadoghq.DatadogInstrumentationLogFields{
							Service: service,
							Source:  logSource,
							Tags:    []string{testTag},
						},
					},
				},
			},
		},
	}, nil
}

func newHTTPCheckInstance(tags []string) (runtime.RawExtension, error) {
	return rawExtension(map[string]any{
		"name":    "DDI E2E Nginx",
		"url":     "http://%%host%%:80/",
		"timeout": 5,
		"tags":    tags,
	})
}

func rawExtension(value any) (runtime.RawExtension, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return runtime.RawExtension{}, err
	}
	return runtime.RawExtension{Raw: raw}, nil
}
