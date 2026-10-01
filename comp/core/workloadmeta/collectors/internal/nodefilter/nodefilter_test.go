// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build nodefilter

package nodefilter

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgerrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// TestLocalNodeName verifies that only otel-agent running in DDOT standalone
// mode without the kubelet collector opt-out selects this collector (Enabled,
// which the kubelet collector steps aside on), and that it then only applies
// on Kubernetes.
func TestLocalNodeName(t *testing.T) {
	tests := []struct {
		name       string
		flavor     string
		standalone bool
		useKubelet bool
		kubernetes bool
		// wantEnabled is whether the kubelet collector must step aside.
		wantEnabled bool
		// wantNodeName is empty when the collector must be disabled.
		wantNodeName string
	}{
		{
			name:   "standalone otel-agent, defaults to nodefilter",
			flavor: flavor.OTelAgent, standalone: true, kubernetes: true,
			wantEnabled: true, wantNodeName: "test-node",
		},
		{
			name:   "not standalone",
			flavor: flavor.OTelAgent, standalone: false, kubernetes: true,
		},
		{
			name:   "not otel-agent",
			flavor: flavor.DefaultAgent, standalone: true, kubernetes: true,
		},
		{
			name:   "opted back out to kubelet",
			flavor: flavor.OTelAgent, standalone: true, useKubelet: true, kubernetes: true,
		},
		{
			// The kubelet collector disables itself off Kubernetes too.
			name:   "not on Kubernetes",
			flavor: flavor.OTelAgent, standalone: true, kubernetes: false,
			wantEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor.SetTestFlavor(t, tt.flavor)
			if tt.kubernetes {
				pkgconfigenv.SetFeatures(t, pkgconfigenv.Kubernetes)
			} else {
				pkgconfigenv.SetFeatures(t)
			}
			// K8S_NODE_NAME is the otelcollector.standalone.node_from_env_var
			// default.
			t.Setenv("K8S_NODE_NAME", "test-node")
			cfg := config.NewMockWithOverrides(t, map[string]interface{}{
				"otel_standalone": tt.standalone,
				"otelcollector.standalone.use_kubelet_collector": tt.useKubelet,
			})

			assert.Equal(t, tt.wantEnabled, Enabled(cfg))

			nodeName, err := localNodeName(cfg)
			if tt.wantNodeName == "" {
				require.Error(t, err)
				assert.True(t, pkgerrors.IsDisabled(err))
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantNodeName, nodeName)
			}
		})
	}
}

// TestLocalNodeName_EnvVarNotSet verifies that a missing node-name env var
// keeps the kubelet collector stepping aside, so pods aren't collected at
// all, and is therefore logged as a warning that names the remedies rather
// than just disabling the collector.
func TestLocalNodeName_EnvVarNotSet(t *testing.T) {
	flavor.SetTestFlavor(t, flavor.OTelAgent)
	pkgconfigenv.SetFeatures(t, pkgconfigenv.Kubernetes)
	t.Setenv("K8S_NODE_NAME", "")
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"otel_standalone": true,
	})
	warnings := captureWarnings(t)

	assert.True(t, Enabled(cfg))

	_, err := localNodeName(cfg)
	require.Error(t, err)
	assert.False(t, pkgerrors.IsDisabled(err))

	output := warnings()
	assert.Contains(t, output, `environment variable "K8S_NODE_NAME"`)
	assert.Contains(t, output, "otelcollector.standalone.use_kubelet_collector")
}

// TestLocalNodeName_CustomEnvVar verifies that the node name is read from
// whichever environment variable otelcollector.standalone.node_from_env_var
// names, mirroring k8sattributesprocessor's configurable node_from_env_var
// filter rather than hardcoding a single env var.
func TestLocalNodeName_CustomEnvVar(t *testing.T) {
	flavor.SetTestFlavor(t, flavor.OTelAgent)
	pkgconfigenv.SetFeatures(t, pkgconfigenv.Kubernetes)
	t.Setenv("K8S_NODE_NAME", "")
	t.Setenv("MY_CUSTOM_NODE_NAME_VAR", "test-node")
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"otel_standalone": true,
		"otelcollector.standalone.node_from_env_var": "MY_CUSTOM_NODE_NAME_VAR",
	})

	nodeName, err := localNodeName(cfg)
	require.NoError(t, err)
	assert.Equal(t, "test-node", nodeName)
}

// standaloneConfig returns a config under which this collector applies, with
// test-node as the local node's name.
func standaloneConfig(t *testing.T) config.Component {
	flavor.SetTestFlavor(t, flavor.OTelAgent)
	pkgconfigenv.SetFeatures(t, pkgconfigenv.Kubernetes)
	t.Setenv("K8S_NODE_NAME", "test-node")
	return config.NewMockWithOverrides(t, map[string]interface{}{
		"otel_standalone": true,
	})
}

// TestStart verifies that Start lists and watches pods scoped to the local
// node, and pushes both listed and later-watched pods to workloadmeta.
func TestStart(t *testing.T) {
	cfg := standaloneConfig(t)
	wlm := mockedWorkloadmeta(t)

	client := fake.NewClientset(podWithContainer("listed-pod", "listed-pod-uid", "listed-container-id"))

	// The fake clientset ignores field selectors, so record the ones the
	// collector asks for instead.
	var mu sync.Mutex
	var fieldSelectors []string
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		defer mu.Unlock()
		fieldSelectors = append(fieldSelectors, action.(k8stesting.ListAction).GetListRestrictions().Fields.String())
		return false, nil, nil
	})

	c := &collector{
		id:      collectorID,
		catalog: workloadmeta.NodeAgent,
		config:  cfg,
		newClient: func(config.Component) (kubernetes.Interface, error) {
			return client, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, c.Start(ctx, wlm))

	assert.EventuallyWithT(t, func(ct *assert.CollectT) {
		_, err := wlm.GetKubernetesPod("listed-pod-uid")
		assert.NoError(ct, err)
	}, eventuallyTimeout, eventuallyInterval)

	mu.Lock()
	assert.Equal(t, []string{"spec.nodeName=test-node"}, fieldSelectors)
	mu.Unlock()

	watchedPod := podWithContainer("watched-pod", "watched-pod-uid", "watched-container-id")
	_, err := client.CoreV1().Pods(watchedPod.Namespace).Create(ctx, watchedPod, metav1.CreateOptions{})
	require.NoError(t, err)

	assert.EventuallyWithT(t, func(ct *assert.CollectT) {
		_, err := wlm.GetKubernetesPod("watched-pod-uid")
		assert.NoError(ct, err)
		_, err = wlm.GetContainer("watched-container-id")
		assert.NoError(ct, err)
	}, eventuallyTimeout, eventuallyInterval)
}

// TestStart_ClientError verifies that failing to build the API client fails
// Start with an error workloadmeta retries, rather than one that drops the
// collector for good.
func TestStart_ClientError(t *testing.T) {
	c := &collector{
		id:      collectorID,
		catalog: workloadmeta.NodeAgent,
		config:  standaloneConfig(t),
		newClient: func(config.Component) (kubernetes.Interface, error) {
			return nil, errors.New("no in-cluster config")
		},
	}

	err := c.Start(context.Background(), nil)
	require.Error(t, err)
	assert.True(t, retry.IsErrWillRetry(err))
}

// syncBuffer is a bytes.Buffer safe to log to from the reflector's goroutine
// while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureWarnings routes the global logger's warnings and errors to a buffer
// until the test ends, and returns a function that flushes and reads it.
// Anything else that sets up the global logger, like the workloadmeta mock,
// must do so before.
func captureWarnings(t *testing.T) func() string {
	var output syncBuffer
	logger, err := log.LoggerFromWriterWithMinLevel(&output, log.WarnLvl)
	require.NoError(t, err)
	t.Cleanup(func() {
		log.SetupLogger(log.Default(), log.InfoStr)
		logger.Close()
	})
	log.SetupLogger(logger, log.WarnStr)

	return func() string {
		logger.Flush()
		return output.String()
	}
}

// TestStart_Forbidden verifies that the API server refusing to list pods, as
// it does when the agent lacks RBAC for them, is logged to the agent's own log
// with a hint, since Start itself has already succeeded by then.
func TestStart_Forbidden(t *testing.T) {
	wlm := mockedWorkloadmeta(t)
	warnings := captureWarnings(t)

	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("RBAC: access denied"))
	})

	c := &collector{
		id:      collectorID,
		catalog: workloadmeta.NodeAgent,
		config:  standaloneConfig(t),
		newClient: func(config.Component) (kubernetes.Interface, error) {
			return client, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, c.Start(ctx, wlm))

	assert.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.Contains(ct, warnings(), "grant the agent's service account list and watch on pods")
	}, eventuallyTimeout, eventuallyInterval)
}
