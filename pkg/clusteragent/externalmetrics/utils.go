// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"

	"k8s.io/apimachinery/pkg/labels"
)

const (
	autogenDatadogMetricPrefix string = "dcaautogen-"
	datadogMetricRefPrefix     string = "datadogmetric@"
	datadogMetricRefSep        string = ":"
	kubernetesNameFormat       string = "([a-z0-9](?:[-a-z0-9]*[a-z0-9])?)"
	kubernetesNamespaceSep     string = "/"
)

var (
	datadogMetricFormat regexp.Regexp = *regexp.MustCompile("^" + datadogMetricRefPrefix + kubernetesNameFormat + datadogMetricRefSep + kubernetesNameFormat + "$")
)

// datadogMetric.ID is namespace/name
func metricNameToDatadogMetricID(metricName string) (string, bool, bool) {
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
	// We keep 20 bytes (160 bits), it should provide a 40-chars hex string
	sum := sha256.Sum256([]byte(query))
	return autogenDatadogMetricPrefix + hex.EncodeToString(sum[0:20])
}

func buildDatadogQueryForExternalMetric(metricName string, labels map[string]string) string {
	aggregator := config.Datadog.GetString("external_metrics.aggregator")
	rollup := config.Datadog.GetInt("external_metrics_provider.rollup")
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

	return fmt.Sprintf("%s:%s.rollup(%d)", aggregator, result, rollup)
}
