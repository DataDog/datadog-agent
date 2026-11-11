// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// newKubeCapabilitiesTest returns a discovery client for a fake API server whose
// /version endpoint fails versionFailures times (forever if negative) and whose
// /metrics endpoint is served by metrics, and a workloadmeta store.
func newKubeCapabilitiesTest(t *testing.T, versionFailures int32, metrics http.HandlerFunc) (discovery.DiscoveryInterface, workloadmetamock.Mock) {
	// KubeServerVersion caches the version globally.
	cache.Cache.Flush()
	t.Cleanup(cache.Cache.Flush)
	previousTimeout, previousBackoff := kubeCapabilitiesTimeout, kubeCapabilitiesBackoff
	kubeCapabilitiesTimeout, kubeCapabilitiesBackoff = 100*time.Millisecond, wait.Backoff{Duration: 10 * time.Millisecond}
	t.Cleanup(func() {
		kubeCapabilitiesTimeout, kubeCapabilitiesBackoff = previousTimeout, previousBackoff
	})

	var versionCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			if versionFailures < 0 || versionCalls.Add(1) <= versionFailures {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"major":"1","minor":"22","gitVersion":"v1.22.17"}`))
		case "/metrics":
			metrics(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	discoveryCl, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)

	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	return discoveryCl, wmeta
}

func metricsHandler(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte("# TYPE kubernetes_feature_enabled gauge\nkubernetes_feature_enabled{name=\"SomeFeature\",stage=\"BETA\"} 1\n"))
		}
	}
}

func TestCollectKubeCapabilities(t *testing.T) {
	tests := []struct {
		name             string
		versionFailures  int32
		metricsStatus    int
		wantFeatureGates map[string]workloadmeta.FeatureGate
	}{
		{
			name:          "feature gates collected",
			metricsStatus: http.StatusOK,
			wantFeatureGates: map[string]workloadmeta.FeatureGate{
				"SomeFeature": {Name: "SomeFeature", Stage: workloadmeta.StageBeta, Enabled: true},
			},
		},
		{
			name:          "no access to metrics",
			metricsStatus: http.StatusForbidden,
		},
		{
			name:            "version collected after a failure",
			versionFailures: 1,
			metricsStatus:   http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discoveryCl, wmeta := newKubeCapabilitiesTest(t, tt.versionFailures, metricsHandler(tt.metricsStatus))

			// The version is retried until the context ends.
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			collectKubeCapabilities(ctx, discoveryCl, wmeta)

			kubeCapabilities, err := wmeta.GetKubeCapabilities()
			require.NoError(t, err)
			require.NotNil(t, kubeCapabilities.Version)
			assert.Equal(t, "v1.22.17", kubeCapabilities.Version.GitVersion)
			assert.Equal(t, tt.wantFeatureGates, kubeCapabilities.FeatureGates)
		})
	}
}

func TestCollectKubeCapabilitiesVersionNeverCollected(t *testing.T) {
	discoveryCl, wmeta := newKubeCapabilitiesTest(t, -1, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	collectKubeCapabilities(ctx, discoveryCl, wmeta)

	_, err := wmeta.GetKubeCapabilities()
	assert.Error(t, err)
}

func TestCollectKubeCapabilitiesPublishesVersionBeforeFeatureGates(t *testing.T) {
	release := make(chan struct{})
	discoveryCl, wmeta := newKubeCapabilitiesTest(t, 0, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
		metricsHandler(http.StatusOK)(w, r)
	})
	// Increase the timeout so that feature gates are still pending while checking the version.
	kubeCapabilitiesTimeout = 5 * time.Second

	done := make(chan struct{})
	go func() {
		defer close(done)
		collectKubeCapabilities(context.Background(), discoveryCl, wmeta)
	}()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		kubeCapabilities, err := wmeta.GetKubeCapabilities()
		require.NoError(c, err)
		require.NotNil(c, kubeCapabilities.Version)
		assert.Equal(c, "v1.22.17", kubeCapabilities.Version.GitVersion)
		assert.Nil(c, kubeCapabilities.FeatureGates)
	}, 2*time.Second, 10*time.Millisecond)

	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("collectKubeCapabilities did not return after feature gates were served")
	}
	kubeCapabilities, err := wmeta.GetKubeCapabilities()
	require.NoError(t, err)
	assert.Contains(t, kubeCapabilities.FeatureGates, "SomeFeature")
}
