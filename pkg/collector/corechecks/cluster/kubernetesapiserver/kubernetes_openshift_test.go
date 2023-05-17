// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	osq "github.com/openshift/api/quota/v1"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestReportClusterQuotas(t *testing.T) {
	raw, err := os.ReadFile("./testdata/oshift_crq_list.json")
	require.NoError(t, err)
	list := osq.ClusterResourceQuotaList{}
	json.Unmarshal(raw, &list)
	require.Len(t, list.Items, 1)

	prevClusterName := config.Datadog.GetString("cluster_name")
	config.Datadog.Set("cluster_name", "test-cluster-name")
	defer config.Datadog.Set("cluster_name", prevClusterName)

	instanceCfg := []byte("")
	initCfg := []byte("")
	kubeASCheck := KubernetesASFactory().(*KubeASCheck)
	err = kubeASCheck.Configure(integration.FakeConfigHash, instanceCfg, initCfg, "test")
	require.NoError(t, err)

	mocked := mocksender.NewMockSender(kubeASCheck.ID())
	mocked.SetupAcceptAll()
	kubeASCheck.reportClusterQuotas(list.Items, mocked)
	mocked.AssertNumberOfCalls(t, "Gauge", 9*3)

	// Total
	expectedTags := []string{"clusterquota:multiproj-test"}

	mocked.AssertMetric(t, "Gauge", "openshift.clusterquota.cpu.limit", 3.0, "", expectedTags)
	mocked.AssertMetric(t, "Gauge", "openshift.clusterquota.cpu.used", 0.6, "", expectedTags)
	mocked.AssertMetric(t, "Gauge", "openshift.clusterquota.cpu.remaining", 2.4, "", expectedTags)

	mocked.AssertMetric(t, "Gauge", "openshift.clusterquota.pods.limit", 10, "", expectedTags)
	mocked.AssertMetric(t, "Gauge", "openshift.clusterquota.pods.used", 6, "", expectedTags)
	mocked.AssertMetric(t, "Gauge", "openshift.clusterquota.pods.remaining", 4, "", expectedTags)

	mocked.AssertMetric(t, "Gauge", "openshift.clusterquota.secrets.limit", 30, "", expectedTags)
	mocked.AssertMetric(t, "Gauge", "openshift.clusterquota.secrets.used", 18, "", expectedTags)
	mocked.AssertMetric(t, "Gauge", "openshift.clusterquota.secrets.remaining", 12, "", expectedTags)

	// Proj1
	proj1Tags := append(expectedTags, "kube_namespace:proj1")

	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.cpu.limit", 3.0, "", proj1Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.cpu.used", 0.6, "", proj1Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.cpu.remaining", 2.4, "", proj1Tags)

	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.pods.limit", 10, "", proj1Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.pods.used", 6, "", proj1Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.pods.remaining", 4, "", proj1Tags)

	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.secrets.limit", 30, "", proj1Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.secrets.used", 9, "", proj1Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.secrets.remaining", 12, "", proj1Tags)

	// Proj2
	proj2Tags := append(expectedTags, "kube_namespace:proj2")

	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.cpu.limit", 3.0, "", proj2Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.cpu.used", 0, "", proj2Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.cpu.remaining", 2.4, "", proj2Tags)

	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.pods.limit", 10, "", proj2Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.pods.used", 0, "", proj2Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.pods.remaining", 4, "", proj2Tags)

	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.secrets.limit", 30, "", proj2Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.secrets.used", 9, "", proj2Tags)
	mocked.AssertMetric(t, "Gauge", "openshift.appliedclusterquota.secrets.remaining", 12, "", proj2Tags)

	if t.Failed() {
		// Debug output
		for i, call := range mocked.Calls {
			fmt.Printf("Call %d: %+v\n", i, call)
		}
	}
}
