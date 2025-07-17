// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver && local_debug

package autoscalers

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"

	datadogclientimpl "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/impl"
)

var (
	kubeClusterName = ""
	queries         = []string{}
)

// Not run, just for local testing
func TestFromLocal(t *testing.T) {
	t.Log("Running test from local")

	endpoint := "https://api.datadoghq.com"
	apiKey := ""
	appKey := ""

	// Configure from env
	if testEndpoint := os.Getenv("DD_ENDPOINT"); testEndpoint != "" {
		endpoint = testEndpoint
	}

	if testAPIKey := os.Getenv("DD_API_KEY"); testAPIKey != "" {
		apiKey = testAPIKey
	}

	if testAppKey := os.Getenv("DD_APP_KEY"); testAppKey != "" {
		appKey = testAppKey
	}

	if apiKey == "" || appKey == "" {
		t.Fatal("API key or app key not set")
	}

	// Config and logger
	cfg := config.NewMock(t)
	logger := logmock.New(t)

	cfg.Set("external_metrics_provider.enabled", true, pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("external_metrics_provider.endpoint", endpoint, pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("external_metrics_provider.api_key", apiKey, pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("external_metrics_provider.app_key", appKey, pkgconfigmodel.SourceLocalConfigProcess)

	datadogClient, err := datadogclientimpl.NewComponent(datadogclientimpl.Requires{Config: cfg, Log: logger})
	if err != nil {
		t.Fatalf("Failed to create datadog client: %v", err)
	}

	// Process queries
	for i := range queries {
		queries[i] = strings.ReplaceAll(queries[i], "%%tag_kube_cluster_name%%", kubeClusterName)
	}

	processor := NewProcessor(datadogClient.Comp)
	res := processor.QueryExternalMetric(queries, 5*time.Minute)

	t.Logf("Results: %v", res)
}
