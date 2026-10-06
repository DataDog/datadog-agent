// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

// Package custommetrics runs the Kubernetes custom metrics API server.
package custommetrics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/pflag"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/apiserver"
	basecmd "sigs.k8s.io/custom-metrics-apiserver/pkg/cmd"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/externalmetrics"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
)

var cmd *DatadogMetricsAdapter

var stopCh chan struct{}

// DatadogMetricsAdapter TODO  <container-integrations>
type DatadogMetricsAdapter struct {
	basecmd.AdapterBase
}

const (
	metricsServerConf = "external_metrics_provider.config"
	adapterName       = "datadog-custom-metrics-adapter"
	adapterVersion    = "1.0.0"
	tlsVersion13Str   = "VersionTLS13"
)

// RunServer creates and start a k8s custom metrics API server
func RunServer(ctx context.Context, apiCl *as.APIClient, datadogCl option.Option[datadogclient.Component]) error {
	defer clearServerResources()
	if apiCl == nil {
		return errors.New("unable to run server with nil APIClient")
	}

	cmd = &DatadogMetricsAdapter{}
	cmd.Name = adapterName
	cmd.FlagSet = pflag.NewFlagSet(cmd.Name, pflag.ExitOnError)

	var c []string
	for k, v := range pkgconfigsetup.Datadog().GetStringMapString(metricsServerConf) {
		c = append(c, fmt.Sprintf("--%s=%s", k, v))
	}

	if err := cmd.Flags().Parse(c); err != nil {
		return err
	}

	server, stopProvider, err := buildServerWithRetry(ctx, apiCl, datadogCl)
	if err != nil {
		return err
	}

	defer stopProvider()

	return server.GenericAPIServer.PrepareRun().RunWithContext(ctx)
}

// buildServerWithRetry constructs the custom metrics API server with exponential backoff.
func buildServerWithRetry(ctx context.Context, apiCl *as.APIClient, datadogCl option.Option[datadogclient.Component]) (*apiserver.CustomMetricsAdapterServer, func(), error) {
	retries := pkgconfigsetup.Datadog().GetInt("external_metrics_provider.startup_retries")
	retryDelay := pkgconfigsetup.Datadog().GetDuration("external_metrics_provider.startup_retry_delay")
	retryMaxDelay := pkgconfigsetup.Datadog().GetDuration("external_metrics_provider.startup_retry_max_delay")
	if retries < 1 {
		return nil, nil, fmt.Errorf("external_metrics_provider.startup_retries must be a positive integer, got %d", retries)
	}
	if retryDelay <= 0 {
		return nil, nil, fmt.Errorf("external_metrics_provider.startup_retry_delay must be a positive duration, got %s", retryDelay)
	}
	if retryMaxDelay <= 0 {
		return nil, nil, fmt.Errorf("external_metrics_provider.startup_retry_max_delay must be a positive duration, got %s", retryMaxDelay)
	}
	if retryMaxDelay < retryDelay {
		return nil, nil, fmt.Errorf("external_metrics_provider.startup_retry_max_delay (%s) must be greater than or equal to startup_retry_delay (%s)", retryMaxDelay, retryDelay)
	}

	backoff := wait.Backoff{
		Steps:    retries,
		Duration: retryDelay,
		Factor:   2.0,
		Jitter:   0.1,
		Cap:      retryMaxDelay,
	}

	var deps apiServerDeps
	if err := retrySetup(ctx, backoff, func(_ context.Context) error {
		var err error
		deps, err = cmd.setupAPIServerDeps(apiCl)
		return err
	}); err != nil {
		return nil, nil, err
	}

	// The Datadog Metric Provider uses shared informers so it is constructed exactly once.
	providerCtx, cancelProvider := context.WithCancel(ctx)
	provider, err := cmd.buildProvider(providerCtx, apiCl, datadogCl, deps)
	if err != nil {
		cancelProvider()
		return nil, nil, err
	}
	cmd.WithExternalMetrics(provider)

	conf, err := cmd.Config()
	if err != nil {
		cancelProvider()
		return nil, nil, err
	}
	server, err := conf.Complete(nil).New(cmd.Name, nil, provider)
	if err != nil {
		cancelProvider()
		return nil, nil, err
	}
	return server, cancelProvider, nil
}

// retrySetup runs setup with exponential backoff until it succeeds, the context
// is cancelled, or the attempt budget is spent. The backoff must be validated by
// the caller; this function assumes backoff.Steps >= 1 and a positive Duration.
func retrySetup(ctx context.Context, backoff wait.Backoff, setup func(context.Context) error) error {
	attempts := backoff.Steps
	delay := backoff.Duration

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := setup(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt == attempts {
			break
		}
		delayBeforeRetry := delay
		if backoff.Jitter > 0 {
			delayBeforeRetry = wait.Jitter(delay, backoff.Jitter)
		}
		log.Warnf("External Metrics Provider setup attempt %d/%d failed, will retry in %s: %v", attempt, attempts, delayBeforeRetry, err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delayBeforeRetry):
		}

		// Grow the delay by Factor and clamp to Cap without ending the loop early.
		if backoff.Factor > 0 {
			delay = time.Duration(float64(delay) * backoff.Factor)
			if backoff.Cap > 0 && delay > backoff.Cap {
				delay = backoff.Cap
			}
		}
	}

	return fmt.Errorf("setup failed after %d attempts: %w", attempts, lastErr)
}

// apiServerDeps holds the APIServer-dependent handles gathered during the
// retriable discovery phase. None of these start background goroutines.
type apiServerDeps struct {
	client dynamic.Interface
	mapper apimeta.RESTMapper
	store  custommetrics.Store
	hpaGVR schema.GroupVersionResource
}

// setupAPIServerDeps constructs the dynamic client, REST mapper, configmap store, and HPA GroupVersionResource (if using the CRD provider path).
func (a *DatadogMetricsAdapter) setupAPIServerDeps(apiCl *as.APIClient) (apiServerDeps, error) {
	client, err := a.DynamicClient()
	if err != nil {
		log.Infof("Unable to construct dynamic client: %v", err)
		return apiServerDeps{}, err
	}

	mapper, err := a.RESTMapper()
	if err != nil {
		log.Errorf("Unable to construct discovery REST mapper: %v", err)
		return apiServerDeps{}, err
	}

	// The CRD provider path starts shared informers inside its constructor, so
	// its store handle is unused; only the configmap-backed path needs a store.
	if pkgconfigsetup.Datadog().GetBool("external_metrics_provider.use_datadogmetric_crd") {
		hpaGVR, err := autoscalers.DiscoverHPAGroupVersionResource(apiCl.Cl)
		if err != nil {
			log.Errorf("Unable to discover HPA GroupVersionResource: %v", err)
			return apiServerDeps{}, err
		}
		return apiServerDeps{client: client, mapper: mapper, hpaGVR: hpaGVR}, nil
	}

	datadogHPAConfigMap := custommetrics.GetConfigmapName()
	store, err := custommetrics.NewConfigMapStore(apiCl.Cl, namespace.GetResourcesNamespace(), datadogHPAConfigMap)
	if err != nil {
		log.Errorf("Unable to create ConfigMap Store: %v", err)
		return apiServerDeps{}, err
	}

	return apiServerDeps{client: client, mapper: mapper, store: store}, nil
}

// buildProvider constructs the Datadog Metric Provider.
func (a *DatadogMetricsAdapter) buildProvider(ctx context.Context, apiCl *as.APIClient, datadogCl option.Option[datadogclient.Component], deps apiServerDeps) (provider.ExternalMetricsProvider, error) {
	if pkgconfigsetup.Datadog().GetBool("external_metrics_provider.use_datadogmetric_crd") {
		if dc, ok := datadogCl.Get(); ok {
			return externalmetrics.NewDatadogMetricProvider(ctx, apiCl, dc, deps.hpaGVR)
		}
		return nil, errors.New("unable to create DatadogMetricProvider as DatadogClient failed with uninitialized datadog client")
	}

	return custommetrics.NewDatadogProvider(ctx, deps.client, deps.mapper, deps.store), nil
}

// Config creates the configuration containing the required parameters to communicate with the APIServer as an APIService
func (a *DatadogMetricsAdapter) Config() (*apiserver.Config, error) {
	if !a.FlagSet.Lookup("cert-dir").Changed {
		// Ensure backward compatibility. Was hardcoded before.
		// Config flag is now to be added to the map `external_metrics_provider.config` as, `cert-dir`.
		a.SecureServing.ServerCert.CertDirectory = "/etc/datadog-agent/certificates"
	}
	if !a.FlagSet.Lookup("secure-port").Changed {
		// Ensure backward compatibility. 443 by default, but will error out if incorrectly set.
		// refer to apiserver code in k8s.io/apiserver/pkg/server/option/serving.go
		a.SecureServing.BindPort = pkgconfigsetup.Datadog().GetInt("external_metrics_provider.port")
		// Default in External Metrics is TLS 1.2
		if !pkgconfigsetup.Datadog().GetBool("cluster_agent.allow_legacy_tls") {
			a.SecureServing.MinTLSVersion = tlsVersion13Str
		}
	}

	return a.AdapterBase.Config()
}

// clearServerResources closes the connection and the server
// stops listening to new commands.
func clearServerResources() {
	if stopCh != nil {
		close(stopCh)
	}
}
