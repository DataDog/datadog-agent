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
// read access to pods on the API server, not kubelet API access
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	collectorID   = "nodefilter"
	componentName = "workloadmeta-nodefilter"

	// noResync matches the kubeapiserver collector's own reflector_store.go:
	// Resync() is never called, so the store never needs to implement it.
	noResync = time.Duration(0)

	// informerClientQPS and informerClientBurst match the limits
	// pkg/util/kubernetes/apiserver/apiserver.go gives its own informer
	// clients: a single reflector only issues a list plus periodic re-watches.
	informerClientQPS   = 5
	informerClientBurst = 10
)

type dependencies struct {
	fx.In

	Config config.Component
}

type collector struct {
	id      string
	catalog workloadmeta.AgentType
	config  config.Component

	// newClient builds the clientset the pod reflector lists and watches
	// with: newAPIClient, or a fake clientset in tests.
	newClient func(config.Component) (kubernetes.Interface, error)

	// includeEphemeralContainers mirrors the kubelet collector's own
	// include_ephemeral_containers config key.
	includeEphemeralContainers bool
}

// NewCollector returns a nodefilter CollectorProvider that instantiates its collector
func NewCollector(deps dependencies) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:                         collectorID,
			catalog:                    workloadmeta.NodeAgent,
			config:                     deps.Config,
			newClient:                  newAPIClient,
			includeEphemeralContainers: deps.Config.GetBool("include_ephemeral_containers"),
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

// Enabled reports whether the nodefilter collector applies to cfg. The kubelet
// collector only steps aside for nodefilter when this holds, so that a
// standalone otel-agent nodefilter can't run in (e.g. one whose deployment
// lacks the node-name env var) keeps collecting pods through kubelet instead
// of through nothing. Only config and environment are checked: a nodefilter
// that then fails against the API server (e.g. on missing pods list/watch
// RBAC) doesn't hand back to kubelet.
func Enabled(cfg config.Component) bool {
	_, err := localNodeName(cfg)
	return err == nil
}

// localNodeName returns the name of the node whose pods this collector
// watches, or a disabled error when the collector doesn't apply: it only
// applies to otel-agent running on Kubernetes in DDOT standalone mode, when
// that mode hasn't opted back out to the kubelet collector.
//
// The node name is read from the environment variable named by
// otelcollector.standalone.node_from_env_var, mirroring the
// k8sattributesprocessor's own "node_from_env_var" filter config (config.go,
// FilterConfig) rather than hardcoding a single env var name. It defaults to
// K8S_NODE_NAME, the name the OTel Helm chart and Operator already populate
// via the Kubernetes downward API (fieldRef: spec.nodeName) for exactly this
// purpose.
func localNodeName(cfg config.Component) (string, error) {
	standalone := cfg.GetBool("otel_standalone") && flavor.GetFlavor() == flavor.OTelAgent
	if !standalone || cfg.GetBool("otelcollector.standalone.use_kubelet_collector") {
		return "", errors.NewDisabled(componentName, "collector only applies to otel-agent running in DDOT standalone mode without the kubelet collector opt-out")
	}

	if !env.IsFeaturePresent(env.Kubernetes) {
		return "", errors.NewDisabled(componentName, "Agent is not running on Kubernetes")
	}

	nodeFromEnvVar := cfg.GetString("otelcollector.standalone.node_from_env_var")
	nodeName := os.Getenv(nodeFromEnvVar)
	if nodeName == "" {
		return "", errors.NewDisabled(componentName, fmt.Sprintf("environment variable %q (otelcollector.standalone.node_from_env_var) is not set", nodeFromEnvVar))
	}

	return nodeName, nil
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	nodeName, err := localNodeName(c.config)
	if err != nil {
		return err
	}

	client, err := c.newClient(c.config)
	if err != nil {
		// The kubelet collector has already stepped aside for this one, so
		// leave it in workloadmeta's candidate set rather than dropping it for
		// good. This must be a *retry.Error: workloadmeta gates on
		// retry.IsErrWillRetry, which type-asserts rather than unwrapping.
		return &retry.Error{
			LogicError:    fmt.Errorf("cannot create Kubernetes API client: %w", err),
			RessourceName: componentName,
			RetryStatus:   retry.FailWillRetry,
		}
	}

	fieldSelector := fields.OneTermEqualSelector("spec.nodeName", nodeName).String()

	podListWatch := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, options)
			return pods, warnIfForbidden(err)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			watcher, err := client.CoreV1().Pods(metav1.NamespaceAll).Watch(ctx, options)
			return watcher, warnIfForbidden(err)
		},
	}

	// Let the reflector know whether client supports WatchList semantics, as
	// generated informers do: a bare ListWatch hides it, and the reflector
	// would otherwise wait forever on a client that doesn't (e.g. a fake one).
	podListerWatcher := cache.ToListWatcherWithWatchListSemantics(podListWatch, client)

	podReflector := cache.NewNamedReflector(componentName, podListerWatcher, &corev1.Pod{}, newPodStore(store, c.includeEphemeralContainers), noResync)

	go podReflector.RunWithContext(ctx)

	return nil
}

// warnIfForbidden logs err, and passes it through, when it shows the agent
// lacks RBAC to list or watch pods. The reflector keeps retrying such errors
// in the background, long after Start has succeeded, and only reports them
// through klog, so this is otherwise the one failure that neither fails the
// collector nor shows up in the agent's log.
func warnIfForbidden(err error) error {
	if apierrors.IsForbidden(err) {
		log.Warnf("%s cannot list or watch pods on the local node: grant the agent's service account list and watch on pods, or set otelcollector.standalone.use_kubelet_collector to true: %s", componentName, err)
	}
	return err
}

// newAPIClient builds a Kubernetes clientset, following the same
// in-cluster/kubeconfig bootstrap and informer client tuning as
// pkg/util/kubernetes/apiserver/apiserver.go, minus the cluster-agent-only
// concerns (leader election, CRD/kueue informers) that package also carries
// and that a node-scoped, node-agent-side collector doesn't need.
//
// The client only backs a reflector, so it uses the informer client timeout
// (no timeout by default) rather than kubernetes_apiserver_client_timeout:
// rest.Config.Timeout bounds the whole HTTP request, so a short one would cut
// every long-lived watch short and force constant re-watches.
func newAPIClient(cfg config.Component) (kubernetes.Interface, error) {
	cfgPath := cfg.GetString("kubernetes_kubeconfig_path")

	var clientConfig *rest.Config
	var err error
	if cfgPath == "" {
		clientConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}

		if !cfg.GetBool("kubernetes_apiserver_tls_verify") {
			clientConfig.TLSClientConfig.Insecure = true
		}

		if customCAPath := cfg.GetString("kubernetes_apiserver_ca_path"); customCAPath != "" {
			clientConfig.TLSClientConfig.CAFile = customCAPath
		}
	} else {
		clientConfig, err = clientcmd.BuildConfigFromFlags("", cfgPath)
		if err != nil {
			return nil, err
		}
	}

	if cfg.GetBool("kubernetes_apiserver_use_protobuf") {
		clientConfig.ContentType = "application/vnd.kubernetes.protobuf"
	}

	clientConfig.Timeout = time.Duration(cfg.GetInt64("kubernetes_apiserver_informer_client_timeout")) * time.Second
	clientConfig.QPS = informerClientQPS
	clientConfig.Burst = informerClientBurst
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
