// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logComp "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

type leaderNotifier func() (<-chan struct{}, func() bool)

// Start initializes and starts the proxy injector. Must be run before starting the admission controller as the singleton
// is used in there
func Start(ctx context.Context, logger logComp.Component, datadogConfig config.Component, leaderSub leaderNotifier) error {
	if injector != nil {
		return errors.New("can't start proxy injection twice")
	}

	injectorStartOnce.Do(func() {
		config := appsecconfig.FromComponent(datadogConfig, logger)
		if !config.Injection.Enabled && !config.Product.Enabled {
			logger.Debug("Appsec proxy injection is disabled")
			return
		}
		injector = newSecurityInjector(ctx, logger, config, leaderSub)
		if injector == nil {
			return
		}

		logger.Infof("Starting appsec proxy injector with config: %#v", injector.config)
		for typ, pattern := range injector.patterns {
			if _, enabled := config.Proxies[typ]; enabled {
				go injector.run(ctx, typ, pattern)
			} else {
				go cleanupPattern(ctx, logger, injector.k8sClient, pattern)
			}
		}
	})

	return nil
}

func detectProxiesInCluster(ctx context.Context, cl *apiserver.APIClient, logger logComp.Component) (map[appsecconfig.ProxyType]struct{}, error) {
	detected := make(map[appsecconfig.ProxyType]struct{})
	for proxy, detector := range proxyDetectionMap {
		found, err := detector(ctx, cl.DynamicCl)
		if err != nil {
			logger.Debugf("error detecting proxy %s in cluster: %s", proxy, err)
			continue
		}
		if found {
			detected[proxy] = struct{}{}
		}
	}

	return detected, nil
}

type securityInjector struct {
	k8sClient dynamic.Interface
	logger    logComp.Component
	config    appsecconfig.Config
	recorder  record.EventRecorder
	patterns  map[appsecconfig.ProxyType]appsecconfig.InjectionPattern

	leaderSub leaderNotifier
}

// newSecurityInjector initializes and returns a new patcher with a dynamic k8s client
func newSecurityInjector(ctx context.Context, logger logComp.Component, config appsecconfig.Config, leaderSub leaderNotifier) *securityInjector {
	// Get API client for proxy detection and event recording
	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		logger.Errorf("Failed to get API client: %v", err)
		return nil
	}

	if config.AutoDetect {
		detectedProxies, err := detectProxiesInCluster(ctx, apiClient, logger)
		if err != nil {
			logger.Debug("error detecting proxies in cluster: %s", err)
		}
		logger.Debugf("Detected proxies in cluster: %v", slices.Collect(maps.Keys(detectedProxies)))
		maps.Copy(config.Proxies, detectedProxies)
	}

	// Cleanup resources of disabled proxies
	if len(config.Proxies) == 0 {
		logger.Debug("No appsec proxies enabled for injection")
		return nil
	}

	// Create event recorder
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{
		Interface: apiClient.Cl.CoreV1().Events(v1.NamespaceAll),
	})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{
		Component: "datadog-appsec-injector",
	})

	return &securityInjector{
		k8sClient: apiClient.DynamicCl,
		logger:    logger,
		config:    config,
		recorder:  eventRecorder,
		patterns:  instantiatePatterns(config, logger, apiClient.DynamicCl, eventRecorder),

		leaderSub: leaderSub,
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
	typ workItemType
	obj *unstructured.Unstructured
}

func (si *securityInjector) run(ctx context.Context, proxyType appsecconfig.ProxyType, pattern appsecconfig.InjectionPattern) {
	defer si.logger.Info("Shutting down security injector for proxy type ", proxyType)

	if err := pattern.IsInjectionPossible(ctx); err != nil {
		si.logger.Errorf("injection not possible for proxy type %q: %s", proxyType, err)
		return
	}

	leaderNotifChange, isLeader := si.leaderSub()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	health := health.RegisterLiveness("appsec-injector-" + string(proxyType))
	defer func() {
		if err := health.Deregister(); err != nil {
			si.logger.Warnf("error deregistering healthcheck: %s", err)
		}
	}()

	go func() {
		for {
			select {
			case <-health.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		// Check if we should stop
		select {
		case <-ctx.Done():
			return
		default:
		}

		for !isLeader() {
			// Wait to become leader
			select {
			case <-leaderNotifChange:
			case <-ctx.Done():
				return
			}
		}

		if err := si.runLeader(ctx, proxyType, pattern, isLeader); err != nil {
			return
		}
	}
}

func (si *securityInjector) runLeader(ctx context.Context, proxyType appsecconfig.ProxyType, pattern appsecconfig.InjectionPattern, isLeader func() bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	si.logger.Infof("Starting security injector for proxy type %q", proxyType)

	queue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.NewTypedItemExponentialFailureRateLimiter[workItem](
			si.config.BaseBackoff,
			si.config.MaxBackoff,
		),
		workqueue.TypedRateLimitingQueueConfig[workItem]{
			Name:            "appsec_injector_" + string(proxyType),
			MetricsProvider: workqueuetelemetry.NewQueueMetricsProvider(),
		},
	)
	defer queue.ShutDown()

	informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(si.k8sClient, 0, pattern.Namespace(), func(options *metav1.ListOptions) {
		options.LabelSelector = selector.String()
	})

	informer := informerFactory.ForResource(pattern.Resource()).Informer()

	handle, err := informer.AddEventHandler(si.createEventHandler(queue))
	if err != nil {
		si.logger.Warnf("error adding event handler for resource %s: %s", proxyType, err)
		return err
	}

	go informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), handle.HasSynced) {
		si.logger.Warnf("timed out waiting for informer caches to sync for resource %s", proxyType)
		return fmt.Errorf("timed out waiting for informer caches to sync for resource %s", proxyType)
	}

	si.logger.Debug("Watching resource as leader:", proxyType)

	for quit := false; !quit && isLeader(); {
		quit = si.processWorkItem(ctx, proxyType, pattern, queue)
	}

	return nil
}

func (si *securityInjector) createEventHandler(queue workqueue.TypedRateLimitingInterface[workItem]) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			unstructured, ok := obj.(*unstructured.Unstructured)
			if !ok {
				si.logger.Warnf("event handler for unexpected type: %T", obj)
			}
			queue.Add(workItem{obj: unstructured, typ: workItemAdded})
		},
		DeleteFunc: func(obj any) {
			unstructured, ok := obj.(*unstructured.Unstructured)
			if !ok {
				si.logger.Warnf("event handler for unexpected type: %T", obj)
			}
			queue.Add(workItem{obj: unstructured, typ: workItemDeleted})
		},
	}
}

func (si *securityInjector) processWorkItem(ctx context.Context, proxyType appsecconfig.ProxyType,
	pattern appsecconfig.InjectionPattern,
	queue workqueue.TypedRateLimitingInterface[workItem],
) bool {
	select {
	case <-ctx.Done():
		return true
	default:
	}

	item, quit := queue.Get()
	if quit {
		return true
	}

	defer queue.Done(item)

	var err error
	switch item.typ {
	case workItemAdded:
		err = pattern.Added(ctx, item.obj)
	case workItemDeleted:
		err = pattern.Deleted(ctx, item.obj)
	}

	watchedChangesCounter.Inc(string(proxyType), item.typ.String(), strconv.FormatBool(err == nil))

	if err == nil {
		queue.Forget(item)
	} else if queue.NumRequeues(item) < 5 {
		si.logger.Debugf("requeuing item after error: %v", err)
		queue.AddRateLimited(item)
	} else {
		si.logger.Warnf("unable to process item: %v", err)
		queue.Forget(item)
	}

	return false
}

func instantiatePatterns(config appsecconfig.Config, logger logComp.Component, k8sClient dynamic.Interface, recorder record.EventRecorder) map[appsecconfig.ProxyType]appsecconfig.InjectionPattern {
	patterns := make(map[appsecconfig.ProxyType]appsecconfig.InjectionPattern, len(config.Proxies))
	for _, proxy := range appsecconfig.AllProxyTypes {
		constructor, ok := proxyConstructorMap[proxy]
		if !ok {
			logger.Warnf("unknown proxy type for appsec injector: %s", proxy)
			continue
		}

		// Add the proxy type to the common annotations so that it is available in the pattern
		config := config
		config.Injection.CommonAnnotations = maps.Clone(config.Injection.CommonAnnotations)
		config.Injection.CommonAnnotations[appsecconfig.AppsecProcessorProxyTypeAnnotation] = string(proxy)

		pattern := constructor(k8sClient, logger, config, recorder)
		patterns[proxy] = pattern
	}

	return patterns
}

// GetSidecarPatterns returns all patterns that are in SIDECAR mode
// This is used by the admission controller to register the appsec sidecar webhook
func GetSidecarPatterns() []appsecconfig.SidecarInjectionPattern {
	if injector == nil {
		log.Error("Appsec Injector not initialized, cannot setup sidecar patterns")
		return nil
	}

	var sidecarPatterns []appsecconfig.SidecarInjectionPattern

	// Only return patterns for enabled proxies
	for proxyType, pattern := range injector.patterns {
		// Check if pattern is in SIDECAR mode and implements SidecarInjectionPattern
		if _, enabled := injector.config.Proxies[proxyType]; enabled && pattern.Mode() == appsecconfig.InjectionModeSidecar {
			if sidecarPattern, ok := pattern.(appsecconfig.SidecarInjectionPattern); ok {
				sidecarPatterns = append(sidecarPatterns, sidecarPattern)
				injector.logger.Debugf("Gathering sidecar pattern for proxy type: %s", proxyType)
			}
		}
	}

	return sidecarPatterns
}
