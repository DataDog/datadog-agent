// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider/defaults"

	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
)

const (
	autogenExpirationPeriodHours int64 = 3
)

const (
	// dpaTargetReplicasMetric is the external metric name for the DPA horizontal target replicas.
	// HPA config: type: External, metric.name: dpa-target-replicas, selector: {kind: <kind>, name: <targetRef.name>}
	dpaTargetReplicasMetric = "dpa-target-replicas"
)

type datadogMetricProvider struct {
	// Default implementation recommended for ListAllExternalMetrics
	defaults.DefaultExternalMetricsProvider

	apiCl                 *apiserver.APIClient
	store                 DatadogMetricsInternalStore
	autogenNamespace      string
	scalingValuesProvider workload.ScalingValuesProvider
}

var (
	metricsMaxAge              int64
	metricsQueryValidityPeriod int64
)

// NewDatadogMetricProvider configures and returns a new datadogMetricProvider.
// scalingValuesProvider is optional; when non-nil it enables serving the "dpa-target-replicas" metric.
func NewDatadogMetricProvider(ctx context.Context, apiCl *apiserver.APIClient, datadogClient datadogclient.Component, hpaGVR schema.GroupVersionResource, scalingValuesProvider workload.ScalingValuesProvider) (provider.ExternalMetricsProvider, error) {
	if apiCl == nil {
		return nil, errors.New("Impossible to create DatadogMetricProvider without valid APIClient")
	}

	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return nil, fmt.Errorf("Unable to create DatadogMetricProvider as LeaderElection failed with: %v", err)
	}

	aggregator := pkgconfigsetup.Datadog().GetString("external_metrics.aggregator")
	rollup := pkgconfigsetup.Datadog().GetInt("external_metrics_provider.rollup")
	setQueryConfigValues(aggregator, rollup)

	refreshPeriod := pkgconfigsetup.Datadog().GetInt64("external_metrics_provider.refresh_period")
	metricsMaxAge = int64(math.Max(pkgconfigsetup.Datadog().GetFloat64("external_metrics_provider.max_age"), float64(3*rollup)))
	metricsQueryValidityPeriod = int64(pkgconfigsetup.Datadog().GetFloat64("external_metrics_provider.query_validity_period"))
	splitBatchBackoffOnErrors := pkgconfigsetup.Datadog().GetBool("external_metrics_provider.split_batches_with_backoff")
	autogenNamespace := namespace.GetResourcesNamespace()
	autogenEnabled := pkgconfigsetup.Datadog().GetBool("external_metrics_provider.enable_datadogmetric_autogen")
	wpaEnabled := pkgconfigsetup.Datadog().GetBool("external_metrics_provider.wpa_controller")
	numWorkers := pkgconfigsetup.Datadog().GetInt("external_metrics_provider.num_workers")

	provider := &datadogMetricProvider{
		apiCl:                 apiCl,
		store:                 NewDatadogMetricsInternalStore(),
		autogenNamespace:      autogenNamespace,
		scalingValuesProvider: scalingValuesProvider,
	}

	// Start MetricsRetriever, only leader will do refresh metrics
	metricsRetriever, err := NewMetricsRetriever(refreshPeriod, metricsMaxAge, autoscalers.NewProcessor(datadogClient), le.IsLeader, &provider.store, splitBatchBackoffOnErrors)
	if err != nil {
		return nil, fmt.Errorf("Unable to create DatadogMetricProvider as MetricsRetriever failed with: %v", err)
	}
	go metricsRetriever.Run(ctx.Done())

	selectorStr := pkgconfigsetup.Datadog().GetString("external_metrics_provider.autoscaler_autogen_label_selector")
	var autoscalerAutogenLabelSelector labels.Selector
	autoscalerAutogenLabelSelector, err = labels.Parse(selectorStr)
	if err != nil {
		return nil, fmt.Errorf("unable to parse external_metrics_provider.autoscaler_autogen_label_selector %q: %v", selectorStr, err)
	}
	if selectorStr != "" {
		log.Infof("Autoscaler autogen label selector configured: %s", selectorStr)
	}

	var wpaInformer dynamicinformer.DynamicSharedInformerFactory
	if wpaEnabled {
		wpaInformer = apiCl.DynamicInformerFactory
	}

	// Start AutoscalerWatcher, only leader will flag DatadogMetrics as Active/Inactive
	// WPAInformerFactory is nil when WPA is not used. AutoscalerWatcher will check value itself.
	autoscalerWatcher, err := NewAutoscalerWatcher(
		refreshPeriod,
		autogenEnabled,
		autogenExpirationPeriodHours,
		autogenNamespace,
		autoscalerAutogenLabelSelector,
		hpaGVR,
		apiCl.InformerFactory,
		wpaInformer,
		le.IsLeader,
		&provider.store,
	)
	if err != nil {
		return nil, fmt.Errorf("Unabled to create DatadogMetricProvider as AutoscalerWatcher failed with: %v", err)
	}

	// We shift controller refresh period from retrieverRefreshPeriod to maximize the probability to have new data from DD
	controller, err := NewDatadogMetricController(apiCl.DynamicCl, apiCl.DynamicInformerFactory, le.IsLeader, &provider.store)
	if err != nil {
		return nil, fmt.Errorf("Unable to create DatadogMetricProvider as DatadogMetric Controller failed with: %v", err)
	}

	// Start informers & controllers (informers can be started multiple times)
	apiCl.DynamicInformerFactory.Start(ctx.Done())
	apiCl.InformerFactory.Start(ctx.Done())

	go autoscalerWatcher.Run(ctx.Done())
	go controller.Run(ctx, numWorkers)

	return provider, nil
}

// GetExternalMetric returns the value of a metric for a given namespace and metric selector
func (p *datadogMetricProvider) GetExternalMetric(_ context.Context, namespace string, metricSelector labels.Selector, info provider.ExternalMetricInfo) (*external_metrics.ExternalMetricValueList, error) {
	startTime := time.Now()
	res, err := p.getExternalMetric(namespace, metricSelector, info, startTime)
	if err != nil {
		convErr := apierr.NewInternalError(err)
		if convErr != nil {
			err = convErr
		}
	}

	setQueryTelemtry("get", startTime, err)
	return res, err
}

func (p *datadogMetricProvider) getExternalMetric(namespace string, metricSelector labels.Selector, info provider.ExternalMetricInfo, time time.Time) (*external_metrics.ExternalMetricValueList, error) {
	log.Debugf("Received external metric query with ns: %s, selector: %s, metricName: %s", namespace, metricSelector.String(), info.Metric)

	// Convert metric name to lower case to allow proper matching (and DD metrics are always lower case)
	info.Metric = strings.ToLower(info.Metric)

	if info.Metric == dpaTargetReplicasMetric {
		return p.getDPATargetReplicas(namespace, metricSelector)
	}

	// If the metric name is already prefixed, we can directly look up metrics in store
	datadogMetricID, parsed, hasPrefix := metricNameToDatadogMetricID(info.Metric, namespace)
	if !hasPrefix {
		datadogMetricID = p.autogenNamespace + kubernetesNamespaceSep + getAutogenDatadogMetricNameFromSelector(info.Metric, metricSelector)
		parsed = true
	}
	if !parsed {
		return nil, log.Warnf("ExternalMetric does not follow DatadogMetric format: %s", info.Metric)
	}

	datadogMetric := p.store.Get(datadogMetricID)
	log.Tracef("DatadogMetric from store: %v", datadogMetric)

	if datadogMetric == nil {
		return nil, log.Warnf("DatadogMetric not found for metric name: %s, datadogmetricid: %s", info.Metric, datadogMetricID)
	}

	externalMetric, err := datadogMetric.ToExternalMetricFormat(info.Metric, metricsMaxAge, time, metricsQueryValidityPeriod)
	if err != nil {
		return nil, err
	}

	return &external_metrics.ExternalMetricValueList{
		Items: []external_metrics.ExternalMetricValue{*externalMetric},
	}, nil
}

// getDPATargetReplicas serves the "dpa-target-replicas" external metric.
// The selector must contain "kind" and "name" labels matching the DPA's spec.targetRef,
// which are the same values as the HPA's spec.scaleTargetRef.kind and .name.
//
// Returns the raw backend recommendation (status.Horizontal.Target.Replicas), which has NOT
// passed through DPA's stabilization windows or rate-limit policies. As a result, the HPA's
// own stabilization window governs timing instead of DPA's configured policies.
//
// TODO: serve horizontalAction.ToReplicas (post-DPA-stabilization) from a dedicated status
// field as "dpa-desired-replicas" so DPA's policies take effect.
func (p *datadogMetricProvider) getDPATargetReplicas(namespace string, metricSelector labels.Selector) (*external_metrics.ExternalMetricValueList, error) {
	if p.scalingValuesProvider == nil {
		return nil, log.Warnf("%s metric requires autoscaling.workload.enabled=true", dpaTargetReplicasMetric)
	}

	targetKind, kindOK := metricSelector.RequiresExactMatch("kind")
	targetName, nameOK := metricSelector.RequiresExactMatch("name")
	if !kindOK || !nameOK {
		return nil, log.Warnf("%s metric requires exact 'kind' and 'name' selector labels matching the HPA's scaleTargetRef, got: %s", dpaTargetReplicasMetric, metricSelector.String())
	}

	replicas, err := p.scalingValuesProvider.TargetReplicas(namespace, targetKind, targetName)
	if err != nil {
		return nil, log.Warnf("%s query failed for %s/%s/%s: %v", dpaTargetReplicasMetric, namespace, targetKind, targetName, err)
	}

	log.Debugf("Serving %s=%d for %s/%s/%s", dpaTargetReplicasMetric, replicas, namespace, targetKind, targetName)

	return &external_metrics.ExternalMetricValueList{
		Items: []external_metrics.ExternalMetricValue{
			{
				MetricName: dpaTargetReplicasMetric,
				Timestamp:  metav1.Now(),
				Value:      *resource.NewQuantity(int64(replicas), resource.DecimalSI),
			},
		},
	}, nil
}
