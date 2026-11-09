// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package instrumentation

import (
	"context"
	"testing"
	"time"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	apm "github.com/DataDog/datadog-agent/comp/instrumentation/apm/def"
	checks "github.com/DataDog/datadog-agent/comp/instrumentation/checks/def"
	"github.com/DataDog/datadog-agent/comp/instrumentation/common"
	controller "github.com/DataDog/datadog-agent/comp/instrumentation/controller/def"
	logs "github.com/DataDog/datadog-agent/comp/instrumentation/logs/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/ssi"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(), telemetrymock.Module())
}

func TestSharedHandlersAndStores(t *testing.T) {
	type dependencies struct {
		fx.In
		Controller     controller.Component
		Checks         checks.Component
		Logs           logs.Component
		APM            apm.Component
		Handlers       []instrumentation.Handler
		CheckStore     *common.CheckStore
		APMStore       *common.APMTargetStore
		TemplateStore  *common.ServiceCheckTemplateStore
		ServiceTracker adtypes.ServiceTracker
	}
	deps := fxutil.Test[dependencies](t, Bundle(), telemetrymock.Module())
	// The admission webhook and controller receive these same component instances,
	// in the original checks, logs, APM order.
	require.Len(t, deps.Handlers, 3)
	assert.Same(t, deps.Checks, deps.Handlers[0])
	assert.Same(t, deps.Logs, deps.Handlers[1])
	assert.Same(t, deps.APM, deps.Handlers[2])
	assert.Same(t, deps.TemplateStore, deps.ServiceTracker)

	cr := &datadoghq.DatadogInstrumentation{
		TypeMeta:   metav1.TypeMeta{APIVersion: datadoghq.GroupVersion.String(), Kind: "DatadogInstrumentation"},
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", UID: "test", Generation: 1},
		Spec: datadoghq.DatadogInstrumentationSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "app"},
			Config: datadoghq.DatadogInstrumentationConfig{
				Checks: []datadoghq.DatadogInstrumentationCheckConfig{{
					Integration: "redisdb", ContainerName: "app", Instances: []runtime.RawExtension{{Raw: []byte(`{"host":"localhost"}`)}},
				}},
				Logs: []datadoghq.DatadogInstrumentationLogConfig{{ContainerName: "app"}},
				APM:  &datadoghq.DatadogInstrumentationAPMConfig{Enabled: true},
			},
		},
	}
	obj := &unstructured.Unstructured{}
	require.NoError(t, instrumentation.UnstructuredFromDatadogInstrumentation(cr, obj))
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		instrumentation.DatadogInstrumentationGVR: "DatadogInstrumentationList",
	}, obj)
	informer := dynamicinformer.NewDynamicSharedInformerFactory(client, 0)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); informer.Shutdown() })
	require.NoError(t, deps.Controller.Start(ctx, controller.Params{
		StatusClient: client, InformerFactory: informer, IsLeader: func() bool { return false },
	}))
	informer.Start(ctx.Done())
	target := ssi.DDICRTarget{Kind: "Deployment", Namespace: "default", Name: "app"}
	require.Eventually(t, func() bool {
		configs, _ := deps.CheckStore.ListConfigs()
		_, hasAPM := deps.APMStore.GetTarget(target)
		return len(configs) == 2 && hasAPM
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, client.Resource(instrumentation.DatadogInstrumentationGVR).Namespace(cr.Namespace).Delete(ctx, cr.Name, metav1.DeleteOptions{}))
	require.Eventually(t, func() bool {
		configs, _ := deps.CheckStore.ListConfigs()
		_, hasAPM := deps.APMStore.GetTarget(target)
		return len(configs) == 0 && !hasAPM
	}, 5*time.Second, 10*time.Millisecond)
}
