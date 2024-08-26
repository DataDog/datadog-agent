// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

// Package custommetrics runs the Kubernetes custom metrics API server.
package custommetrics

import (
	"context"
	"fmt"

	"github.com/spf13/pflag"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/apiserver"
	basecmd "sigs.k8s.io/custom-metrics-apiserver/pkg/cmd"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/externalmetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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
func RunServer(ctx context.Context, apiCl *as.APIClient, datadogCl optional.Option[datadogclient.Component]) error {
	defer clearServerResources()
	if apiCl == nil {
		return fmt.Errorf("unable to run server with nil APIClient")
	}

	cmd = &DatadogMetricsAdapter{}
	cmd.Name = adapterName
	cmd.FlagSet = pflag.NewFlagSet(cmd.Name, pflag.ExitOnError)

	var c []string
	for k, v := range config.Datadog().GetStringMapString(metricsServerConf) {
		c = append(c, fmt.Sprintf("--%s=%s", k, v))
	}

	if err := cmd.Flags().Parse(c); err != nil {
		return err
	}

	provider, err := cmd.makeProviderOrDie(ctx, apiCl, datadogCl)
	if err != nil {
		return err
	}
	cmd.WithExternalMetrics(provider)

	conf, err := cmd.Config()
	if err != nil {
		return err
	}

	server, err := conf.Complete(nil).New(cmd.Name, nil, provider)
	if err != nil {
		return err
	}
	// TODO Add extra logic to only tear down the External Metrics Server if only some components fail.
	return server.GenericAPIServer.PrepareRun().Run(ctx.Done())
}

func (a *DatadogMetricsAdapter) makeProviderOrDie(ctx context.Context, apiCl *as.APIClient, datadogCl optional.Option[datadogclient.Component]) (provider.ExternalMetricsProvider, error) {
	client, err := a.DynamicClient()
	if err != nil {
		log.Infof("Unable to construct dynamic client: %v", err)
		return nil, err
	}

	mapper, err := a.RESTMapper()
	if err != nil {
		log.Errorf("Unable to construct discovery REST mapper: %v", err)
		return nil, err
	}

	if config.Datadog().GetBool("external_metrics_provider.use_datadogmetric_crd") {
		if dc, ok := datadogCl.Get(); ok {
			return externalmetrics.NewDatadogMetricProvider(ctx, apiCl, dc)
		}
		return nil, fmt.Errorf("unable to create DatadogMetricProvider as DatadogClient failed with uninitialized datadog client")
	}

	datadogHPAConfigMap := custommetrics.GetConfigmapName()
	store, err := custommetrics.NewConfigMapStore(apiCl.Cl, common.GetResourcesNamespace(), datadogHPAConfigMap)
	if err != nil {
		log.Errorf("Unable to create ConfigMap Store: %v", err)
		return nil, err
	}

	return custommetrics.NewDatadogProvider(ctx, client, mapper, store), nil
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
		a.SecureServing.BindPort = config.Datadog().GetInt("external_metrics_provider.port")
		// Default in External Metrics is TLS 1.2
		if !config.Datadog().GetBool("cluster_agent.allow_legacy_tls") {
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
