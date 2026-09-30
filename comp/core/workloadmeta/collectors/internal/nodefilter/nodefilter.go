// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build nodefilter

// Package nodefilter implements a workloadmeta Collector that watches pods
// scoped to the local node directly against the Kubernetes API server (via a
// spec.nodeName field selector), mirroring OTel's own k8sattributesprocessor
// "node" filter mode. It exists as a lower-RBAC alternative to the kubelet
// collector for otel-agent running in DDOT standalone mode: it only needs
// read access to pods/nodes on the API server, not kubelet API access
// (nodes/proxy, nodes/stats).
package nodefilter

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	collectorID   = "nodefilter"
	componentName = "workloadmeta-nodefilter"

	// noResync matches the kubeapiserver collector's own reflector_store.go:
	// Resync() is never called, so the store never needs to implement it.
	noResync = time.Duration(0)
)

type dependencies struct {
	fx.In

	Config config.Component
}

type collector struct {
	id      string
	catalog workloadmeta.AgentType
	config  config.Component

	// standalone and useKubelet are computed once at construction time and
	// govern this collector's mutual exclusivity with the kubelet collector:
	// nodefilter only applies to otel-agent running in DDOT standalone mode,
	// and only when that mode hasn't opted back out to kubelet.
	standalone bool
	useKubelet bool

	// nodeFromEnvVar names the environment variable this collector reads the
	// local node's name from, mirroring the k8sattributesprocessor's own
	// "node_from_env_var" filter config (config.go, FilterConfig) rather than
	// hardcoding a single env var name. Defaults to K8S_NODE_NAME, the name
	// the OTel Helm chart and Operator already populate via the Kubernetes
	// downward API (fieldRef: spec.nodeName) for exactly this purpose.
	nodeFromEnvVar string
}

// NewCollector returns a nodefilter CollectorProvider that instantiates its collector
func NewCollector(deps dependencies) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:             collectorID,
			catalog:        workloadmeta.NodeAgent,
			config:         deps.Config,
			standalone:     deps.Config.GetBool("otel_standalone") && flavor.GetFlavor() == flavor.OTelAgent,
			useKubelet:     deps.Config.GetBool("otelcollector.standalone.use_kubelet_collector"),
			nodeFromEnvVar: deps.Config.GetString("otelcollector.standalone.node_from_env_var"),
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if !c.standalone || c.useKubelet {
		return errors.NewDisabled(componentName, "collector only applies to otel-agent running in DDOT standalone mode without the kubelet collector opt-out")
	}

	if !env.IsFeaturePresent(env.Kubernetes) {
		return errors.NewDisabled(componentName, "Agent is not running on Kubernetes")
	}

	nodeName := os.Getenv(c.nodeFromEnvVar)
	if nodeName == "" {
		return errors.NewDisabled(componentName, fmt.Sprintf("environment variable %q (otelcollector.standalone.node_from_env_var) is not set", c.nodeFromEnvVar))
	}

	client, err := newAPIClient(c.config)
	if err != nil {
		return fmt.Errorf("cannot create Kubernetes API client: %w", err)
	}

	fieldSelector := fields.OneTermEqualSelector("spec.nodeName", nodeName).String()

	podListerWatcher := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, options)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return client.CoreV1().Pods(metav1.NamespaceAll).Watch(ctx, options)
		},
	}

	podReflector := cache.NewNamedReflector(componentName, podListerWatcher, &corev1.Pod{}, newPodStore(store), noResync)

	go podReflector.RunWithContext(ctx)

	return nil
}

// newAPIClient builds a Kubernetes clientset, following the same
// in-cluster/kubeconfig bootstrap and client tuning as
// pkg/util/kubernetes/apiserver/apiserver.go, minus the cluster-agent-only
// concerns (leader election, CRD/kueue informers) that package also carries
// and that a node-scoped, node-agent-side collector doesn't need.
func newAPIClient(cfg config.Component) (kubernetes.Interface, error) {
	cfgPath := cfg.GetString("kubernetes_kubeconfig_path")

	var clientConfig *rest.Config
	var err error
	if cfgPath == "" {
		clientConfig, err = rest.InClusterConfig()
	} else {
		clientConfig, err = clientcmd.BuildConfigFromFlags("", cfgPath)
	}
	if err != nil {
		return nil, err
	}

	clientConfig.Timeout = time.Duration(cfg.GetInt64("kubernetes_apiserver_client_timeout")) * time.Second
	clientConfig.QPS = float32(cfg.GetFloat64("kubernetes_apiserver_client_qps"))
	clientConfig.Burst = cfg.GetInt("kubernetes_apiserver_client_burst")
	clientConfig.UserAgent = fmt.Sprintf("datadog-%s/%s", strings.ReplaceAll(flavor.GetFlavor(), "_", "-"), version.AgentVersion)

	return kubernetes.NewForConfig(clientConfig)
}

// Pull is a no-op: the reflector started in Start pushes events
// asynchronously as they happen.
func (c *collector) Pull(_ context.Context) error {
	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}
