// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	workqueuetelemetry "github.com/DataDog/datadog-agent/pkg/util/workqueue/telemetry"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

var (
	injector          *securityInjector
	injectorStartOnce sync.Once

	selector = fields.OneTermNotEqualSelector("appsec.datadoghq.com/enabled", "false")
)

// Start initializes and starts the proxy injector
func Start(ctx context.Context, logger log.Component, datadogConfig config.Component) error {
	if injector != nil {
		return fmt.Errorf("can't start proxy injection twice")
	}

	injectorStartOnce.Do(func() {
		injector = newSecurityInjector(ctx, logger, datadogConfig)
		if injector == nil {
			return
		}

		logger.Infof("Starting appsec proxy injector with config: %#v", injector.config)
		patterns := injector.CompilePatterns()
		for typ, pattern := range patterns {
			go injector.run(typ, pattern)
		}
	})

	return nil
}

func detectProxiesInCluster(ctx context.Context, cl *apiserver.APIClient, logger log.Component) (map[appsecconfig.ProxyType]struct{}, error) {
	detected := make(map[appsecconfig.ProxyType]struct{})
	for proxy, detector := range proxyDetectionMap {
		found, err := detector(ctx, cl.DynamicCl)
		if err != nil {
			logger.Warnf("error detecting proxy %s in cluster: %s", proxy, err)
			continue
		}
		if found {
			detected[proxy] = struct{}{}
		}
	}

	return detected, nil
}

type securityInjector struct {
	ctx                   context.Context
	cancel                context.CancelFunc
	k8sClient             dynamic.Interface
	logger                log.Component
	config                appsecconfig.Config
	recorder              record.EventRecorder
	leaderElectionEnabled bool
	baseBackoff           time.Duration
	maxBackoff            time.Duration
}

// NewLanguagePatcher initializes and returns a new patcher with a dynamic k8s client
func newSecurityInjector(ctx context.Context, logger log.Component, datadogConfig config.Component) *securityInjector {
	config := appsecconfig.FromComponent(datadogConfig)
	if !config.Injection.Enabled && !config.Product.Enabled {
		logger.Info("Appsec proxy injection is disabled")
		return nil
	}

	// Log warning for any unsupported proxy types specified in configuration
	proxiesEnabled := datadogConfig.GetStringSlice("appsec.proxy.proxies")
	for _, p := range proxiesEnabled {
		if _, supported := config.Proxies[appsecconfig.ProxyType(p)]; !supported && p != "" {
			logger.Warnf("Unsupported proxy type %q specified in appsec.proxy.proxies configuration - ignoring", p)
		}
	}

	// Get API client for proxy detection and event recording
	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		logger.Errorf("Failed to get API client: %v", err)
		return nil
	}

	if config.AutoDetect {
		detectedProxies, err := detectProxiesInCluster(ctx, apiClient, logger)
		if err != nil {
			logger.Warnf("error detecting proxies in cluster: %s", err)
		}
		maps.Copy(config.Proxies, detectedProxies)
	}

	if len(config.Proxies) == 0 {
		logger.Info("No appsec proxies enabled for injection")
		return nil
	}

	// Create event recorder
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{
		Interface: apiClient.Cl.CoreV1().Events(config.Processor.Namespace),
	})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{
		Component: "datadog-appsec-injector",
	})

	ctx, cancel := context.WithCancel(ctx)
	return &securityInjector{
		ctx:                   ctx,
		cancel:                cancel,
		k8sClient:             apiClient.DynamicCl,
		logger:                logger,
		config:                config,
		recorder:              eventRecorder,
		leaderElectionEnabled: datadogConfig.GetBool("leader_election"),
		baseBackoff:           datadogConfig.GetDuration("cluster_agent.appsec.injector.base_backoff"),
		maxBackoff:            datadogConfig.GetDuration("cluster_agent.appsec.injector.max_backoff"),
	}
}

type workItemType int

const (
	_ workItemType = iota
	workItemAdded
	workItemDeleted
)

func (sub workItemType) String() string {
	switch sub {
	case workItemAdded:
		return "added"
	case workItemDeleted:
		return "deleted"
	default:
		return "unknown"
	}
}

type workItem struct {
	name      string
	namespace string
	typ       workItemType
}

func (si *securityInjector) run(proxyType appsecconfig.ProxyType, pattern appsecconfig.InjectionPattern) {
	defer si.logger.Info("Shutting down security injector for proxy type ", proxyType)

	if err := pattern.IsInjectionPossible(si.ctx); err != nil {
		si.logger.Errorf("injection not possible for proxy type %q: %s", proxyType, err)
		return
	}

	queue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.NewTypedItemExponentialFailureRateLimiter[workItem](
			si.baseBackoff,
			si.maxBackoff,
		),
		workqueue.TypedRateLimitingQueueConfig[workItem]{
			Name:            "appsec_injector_" + string(proxyType),
			MetricsProvider: workqueuetelemetry.NewQueueMetricsProvider(),
		},
	)

	informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(si.k8sClient, 0, pattern.Namespace(), func(opts *metav1.ListOptions) {
		opts.LabelSelector = selector.String()
	})

	informer := informerFactory.ForResource(pattern.Resource()).Informer()

	handle, err := informer.AddEventHandler(si.createEventHandler(queue))
	if err != nil {
		si.logger.Errorf("error adding event handler for resource %s: %s", proxyType, err)
		return
	}

	go informer.Run(si.ctx.Done())

	if !cache.WaitForCacheSync(si.ctx.Done(), handle.HasSynced) {
		si.logger.Warnf("timed out waiting for informer caches to sync for resource %s", proxyType)
		return
	}

	health := health.RegisterLiveness("appsec-injector-" + string(proxyType))
	defer func() {
		if err := health.Deregister(); err != nil {
			si.logger.Warnf("error deregistering healthcheck: %s", err)
		}
	}()

	si.logger.Debug("Watching resource:", proxyType)

	go func() {
		for {
			select {
			case <-health.C:
			case <-si.ctx.Done():
				queue.ShutDown()
				return
			}
		}
	}()

	for quit := false; !quit; {
		quit = si.processWorkItem(proxyType, pattern, queue)
	}
}

func (si *securityInjector) createEventHandler(queue workqueue.TypedRateLimitingInterface[workItem]) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			unstructured, ok := obj.(*unstructured.Unstructured)
			if !ok {
				si.logger.Warnf("event handler for unexpected type: %T", obj)
			}
			queue.Add(workItem{name: unstructured.GetName(), namespace: unstructured.GetNamespace(), typ: workItemAdded})
		},
		DeleteFunc: func(obj any) {
			unstructured, ok := obj.(*unstructured.Unstructured)
			if !ok {
				si.logger.Warnf("event handler for unexpected type: %T", obj)
			}
			queue.Add(workItem{name: unstructured.GetName(), namespace: unstructured.GetNamespace(), typ: workItemDeleted})
		},
	}
}

func (si *securityInjector) processWorkItem(proxyType appsecconfig.ProxyType,
	pattern appsecconfig.InjectionPattern,
	queue workqueue.TypedRateLimitingInterface[workItem],
) bool {
	item, quit := queue.Get()
	if quit {
		return true
	}

	defer queue.Done(item)

	if !si.isLeader() {
		// Forget the item to prevent retries when we're not the leader
		// Only the leader should process proxy configuration changes
		queue.Forget(item)
		return false
	}

	var err error
	switch item.typ {
	case workItemAdded:
		err = pattern.Added(si.ctx, item.namespace, item.name)
	case workItemDeleted:
		err = pattern.Deleted(si.ctx, item.namespace, item.name)
	}

	watchedChangesCounter.Inc(string(proxyType), item.typ.String(), strconv.FormatBool(err == nil))

	if err == nil {
		queue.Forget(item)
	} else if queue.NumRequeues(item) < 5 {
		si.logger.Debugf("requeuing item after error: %v", err)
		queue.AddRateLimited(item)
	} else {
		si.logger.Warnf("unable to process item: %v", err)
	}

	return false
}

// isLeader checks if the current instance is the leader
func (si *securityInjector) isLeader() bool {
	if !si.leaderElectionEnabled {
		// If leader election is disabled, we're always the leader
		return true
	}

	common.GetResourcesNamespace()

	leaderEngine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		si.logger.Errorf("Failed to get leader engine: %v", err)
		// If we can't determine leader status, don't patch to be safe
		return false
	}

	return leaderEngine.IsLeader()
}

func (si *securityInjector) CompilePatterns() map[appsecconfig.ProxyType]appsecconfig.InjectionPattern {
	patterns := make(map[appsecconfig.ProxyType]appsecconfig.InjectionPattern, len(si.config.Proxies))
	for proxy := range si.config.Proxies {
		constructor, ok := proxyConstructorMap[proxy]
		if !ok {
			si.logger.Warnf("unknown proxy type for appsec injector: %s", proxy)
			continue
		}

		// Add the proxy type to the common annotations so that it is available in the pattern
		config := si.config
		config.Injection.CommonAnnotations = maps.Clone(config.Injection.CommonAnnotations)
		config.Injection.CommonAnnotations[appsecconfig.AppsecProcessorProxyTypeAnnotation] = string(proxy)

		patterns[proxy] = constructor(si.k8sClient, si.logger, config, si.recorder)
		si.logger.Infof("Enabled appsec proxy injection for proxy type %q", proxy)
	}

	return patterns
}
