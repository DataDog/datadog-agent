// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
)

const (
	autogenDatadogMetricPrefix string = "dcaautogen-"
	datadogMetricRefPrefix     string = "datadogmetric@"
	datadogMetricRefSep        string = ":"
	kubernetesNameFormat       string = "([a-z0-9](?:[-a-z0-9]*[a-z0-9])?)"
	kubernetesNamespaceSep     string = "/"
)

var (
	datadogMetricFormat = *regexp.MustCompile("^" + datadogMetricRefPrefix + kubernetesNameFormat + datadogMetricRefSep + kubernetesNameFormat + "$")
	// These values are set by the provider when starting, here are default values for unit tests
	queryConfigAggregator = "avg"
	queryConfigRollup     = 30
)

// datadogMetric.ID is namespace/name
func metricNameToDatadogMetricID(metricName string) (id string, parsed bool, hasPrefix bool) {
	metricName = strings.ToLower(metricName)
	if matches := datadogMetricFormat.FindStringSubmatch(metricName); matches != nil {
		return matches[1] + kubernetesNamespaceSep + matches[2], true, true
	}

	return "", false, strings.HasPrefix(metricName, datadogMetricRefPrefix)
}

func datadogMetricIDToMetricName(datadogMetricID string) string {
	return strings.ToLower(datadogMetricRefPrefix + strings.Replace(datadogMetricID, kubernetesNamespaceSep, datadogMetricRefSep, 1))
}

func getAutogenDatadogMetricNameFromLabels(metricName string, labels map[string]string) string {
	return getAutogenDatadogMetricName(buildDatadogQueryForExternalMetric(metricName, labels))
}

func getAutogenDatadogMetricNameFromSelector(metricName string, labels labels.Selector) string {
	strPairs := strings.Split(labels.String(), ",")
	mapLabels := make(map[string]string, len(strPairs))
	for _, pair := range strPairs {
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			continue
		}

		mapLabels[kv[0]] = kv[1]
	}

	return getAutogenDatadogMetricName(buildDatadogQueryForExternalMetric(metricName, mapLabels))
}

// We use query and not metricName + labels as key. It ensures we'll handle changes of config parameters.
func getAutogenDatadogMetricName(query string) string {
	// We keep 19 bytes (152 bits), it should provide a 38-chars hex string
	// Not using 40chars as it conflicts with appKey scrubbing
	sum := sha256.Sum256([]byte(query))
	return autogenDatadogMetricPrefix + hex.EncodeToString(sum[0:19])
}

func buildDatadogQueryForExternalMetric(metricName string, labels map[string]string) string {
	var result string

	if len(labels) == 0 {
		result = fmt.Sprintf("%s{*}", metricName)
	} else {
		datadogTags := []string{}
		for key, val := range labels {
			datadogTags = append(datadogTags, fmt.Sprintf("%s:%s", key, val))
		}
		sort.Strings(datadogTags)
		tags := strings.Join(datadogTags, ",")
		result = fmt.Sprintf("%s{%s}", metricName, tags)
	}

	return fmt.Sprintf("%s:%s.rollup(%d)", queryConfigAggregator, result, queryConfigRollup)
}

func setQueryConfigValues(aggregator string, rollup int) {
	queryConfigAggregator = aggregator
	queryConfigRollup = rollup
}

// UnstructuredIntoDDM converts an unstructured object into a DatadogMetric
func UnstructuredIntoDDM(obj interface{}, structDest *v1alpha1.DatadogMetric) error {
	unstrObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("Could not cast Unstructured object: %v", obj)
	}
	return runtime.DefaultUnstructuredConverter.FromUnstructured(unstrObj.UnstructuredContent(), structDest)
}

// UnstructuredFromDDM converts a DatadogMetric object into an Unstructured
func UnstructuredFromDDM(structIn *v1alpha1.DatadogMetric, unstructOut *unstructured.Unstructured) error {
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(structIn)
	if err != nil {
		return fmt.Errorf("Unable to convert DatadogMetric %v: %w", structIn, err)
	}
	unstructOut.SetUnstructuredContent(content)
	return nil
}
