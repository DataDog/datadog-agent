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
	"errors"
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
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgerrors "github.com/DataDog/datadog-agent/pkg/errors"
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

	// clientErrorLogged records that a failure to build the API client was
	// already warned about, so that workloadmeta retrying Start doesn't log
	// it again every time. Start is never called concurrently.
	clientErrorLogged bool
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

// Enabled reports whether cfg selects the nodefilter collector, in which case
// the kubelet collector steps aside for it: otel-agent running in DDOT
// standalone mode selects it, unless that mode opted back out to the kubelet
// collector. Only that configuration decides. A nodefilter that then can't
// run, without its node-name env var or refused pods list/watch by the API
// server, doesn't hand back to kubelet: a deployment set up for nodefilter
// lacks the kubelet API access and settings kubelet needs, so kubelet would
// fail too, and only log at debug level while it retries.
func Enabled(cfg config.Component) bool {
	standalone := cfg.GetBool("otel_standalone") && flavor.GetFlavor() == flavor.OTelAgent
	return standalone && !cfg.GetBool("otelcollector.standalone.use_kubelet_collector")
}

// localNodeName returns the name of the node whose pods this collector
// watches and the environment variable it read it from, a disabled error when
// the collector doesn't apply (it isn't selected, or the agent isn't running
// on Kubernetes), or an error when it can't resolve that name.
//
// The node name is read from the first set environment variable among those
// listed, comma-separated, in otelcollector.standalone.node_from_env_var. That
// setting extends the k8sattributesprocessor's own "node_from_env_var" filter
// config (config.go, FilterConfig), which takes a single name, so that the
// name each deployment tool already populates through the Kubernetes downward
// API (fieldRef: spec.nodeName) works without configuration: K8S_NODE_NAME
// (OTel Helm chart presets), DD_KUBERNETES_KUBELET_NODENAME (Datadog Helm
// chart and Operator) and OTEL_K8S_NODE_NAME (OTel Helm chart). When none is
// set, or the value isn't a valid node name once trimmed, no collector gathers
// pods, so this also logs a warning: workloadmeta only logs a collector
// failing to start at info level.
func localNodeName(cfg config.Component) (nodeName, fromEnvVar string, err error) {
	if !Enabled(cfg) {
		return "", "", pkgerrors.NewDisabled(componentName, "collector only applies to otel-agent running in DDOT standalone mode without the kubelet collector opt-out")
	}

	if !env.IsFeaturePresent(env.Kubernetes) {
		return "", "", pkgerrors.NewDisabled(componentName, "Agent is not running on Kubernetes")
	}

	nodeFromEnvVars := cfg.GetString("otelcollector.standalone.node_from_env_var")
	for _, envVar := range strings.Split(nodeFromEnvVars, ",") {
		envVar = strings.TrimSpace(envVar)
		if value := strings.TrimSpace(os.Getenv(envVar)); value != "" {
			nodeName, fromEnvVar = value, envVar
			break
		}
	}
	if nodeName == "" {
		log.Warnf("%s cannot collect pods, so telemetry won't get Kubernetes tags: none of the environment variables in %q (otelcollector.standalone.node_from_env_var) is set. Set one of them to the pod's spec.nodeName through the downward API, or set otelcollector.standalone.use_kubelet_collector to true", componentName, nodeFromEnvVars)
		return "", "", fmt.Errorf("none of the environment variables in %q (otelcollector.standalone.node_from_env_var) is set", nodeFromEnvVars)
	}

	// Node names are DNS subdomains: anything else can't match a pod's
	// spec.nodeName, and the field selector would silently select no pods.
	if problems := validation.IsDNS1123Subdomain(nodeName); len(problems) > 0 {
		log.Warnf("%s cannot collect pods, so telemetry won't get Kubernetes tags: environment variable %q (otelcollector.standalone.node_from_env_var) holds %q, which isn't a valid node name: %s. Set it to the pod's spec.nodeName through the downward API, or set otelcollector.standalone.use_kubelet_collector to true", componentName, fromEnvVar, nodeName, strings.Join(problems, "; "))
		return "", "", fmt.Errorf("environment variable %q (otelcollector.standalone.node_from_env_var) holds an invalid node name %q", fromEnvVar, nodeName)
	}

	return nodeName, fromEnvVar, nil
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	nodeName, nodeEnvVar, err := localNodeName(c.config)
	if err != nil {
		return err
	}

	client, err := c.newClient(c.config)
	if err != nil {
		// workloadmeta only logs retried start failures at debug level, and
		// no other collector gathers pods meanwhile, so warn, once.
		if !c.clientErrorLogged {
			log.Warnf("%s cannot collect pods, so telemetry won't get Kubernetes tags, until it can create a Kubernetes API client: %s. Mount the service account token in the pod or set kubernetes_kubeconfig_path, or set otelcollector.standalone.use_kubelet_collector to true. Will keep retrying", componentName, err)
			c.clientErrorLogged = true
		}

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

	log.Infof("%s watching pods on node %q (read from environment variable %q)", componentName, nodeName, nodeEnvVar)

	fieldSelector := fields.OneTermEqualSelector("spec.nodeName", nodeName).String()
	errs := &listWatchErrors{}

	podListWatch := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, options)
			return pods, errs.report(err)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			watcher, err := client.CoreV1().Pods(metav1.NamespaceAll).Watch(ctx, options)
			return watcher, errs.report(err)
		},
	}

	// Let the reflector know whether client supports WatchList semantics, as
	// generated informers do: a bare ListWatch hides it, and the reflector
	// would otherwise wait forever on a client that doesn't (e.g. a fake one).
	podListerWatcher := cache.ToListWatcherWithWatchListSemantics(podListWatch, client)

	podReflector := cache.NewNamedReflector(componentName, podListerWatcher, &corev1.Pod{}, newPodStore(store, nodeName, c.includeEphemeralContainers), noResync)

	go podReflector.RunWithContext(ctx)

	return nil
}

// listWatchErrors warns about the errors the pod reflector's list and watch
// calls return, with a hint on how to fix them: client-go only logs them to
// stderr through klog, without a hint. The reflector retries for as long as
// they last, so it warns once per hint until a call succeeds. The reflector
// makes these calls one at a time, so this needs no lock.
type listWatchErrors struct {
	// hint is the last one warned about, empty once a call succeeds.
	hint string
}

// report warns about err as described on listWatchErrors, and passes it through.
func (e *listWatchErrors) report(err error) error {
	switch {
	case err == nil:
		e.hint = ""
		return nil
	// The collector is stopping, or the reflector is about to relist on its
	// own: neither needs fixing.
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded),
		apierrors.IsResourceExpired(err), apierrors.IsGone(err):
		return err
	}

	hint := "check that the Kubernetes API server is reachable and that its certificate is trusted (kubernetes_apiserver_ca_path)"
	switch {
	case apierrors.IsForbidden(err):
		hint = "grant the agent's service account list and watch on pods, or set otelcollector.standalone.use_kubelet_collector to true"
	case apierrors.IsUnauthorized(err):
		hint = "check the service account token mounted in the pod, or the credentials in kubernetes_kubeconfig_path"
	}

	if hint != e.hint {
		e.hint = hint
		log.Warnf("%s cannot list or watch pods on the local node, so telemetry won't get Kubernetes tags: %s. Will keep retrying: %s", componentName, hint, err)
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

		applyTLSSettings(cfg, clientConfig)
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

// applyTLSSettings configures how clientConfig, built from the pod's service
// account, verifies the API server's certificate, as
// pkg/util/kubernetes/apiserver/apiserver.go does for its own clients.
func applyTLSSettings(cfg config.Component, clientConfig *rest.Config) {
	if !cfg.GetBool("kubernetes_apiserver_tls_verify") {
		clientConfig.TLSClientConfig.Insecure = true
	}

	if customCAPath := cfg.GetString("kubernetes_apiserver_ca_path"); customCAPath != "" {
		clientConfig.TLSClientConfig.CAFile = customCAPath
	}
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
