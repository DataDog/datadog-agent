// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package custommetrics

import (
	"context"
	"fmt"
	"net"

	"github.com/spf13/pflag"
	openapinamer "k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/apiserver"
	basecmd "sigs.k8s.io/custom-metrics-apiserver/pkg/cmd"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	generatedopenapi "github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics/api/generated/openapi"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
func RunServer(ctx context.Context, apiCl *as.APIClient) error {
	defer clearServerResources()
	if apiCl == nil {
		return fmt.Errorf("unable to run server with nil APIClient")
	}

	cmd = &DatadogMetricsAdapter{}
	cmd.Name = adapterName

	cmd.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(generatedopenapi.GetOpenAPIDefinitions, openapinamer.NewDefinitionNamer(apiserver.Scheme))
	cmd.OpenAPIConfig.Info.Title = adapterName
	cmd.OpenAPIConfig.Info.Version = adapterVersion

	cmd.FlagSet = pflag.NewFlagSet(cmd.Name, pflag.ExitOnError)

	var c []string
	for k, v := range config.Datadog.GetStringMapString(metricsServerConf) {
		c = append(c, fmt.Sprintf("--%s=%s", k, v))
	}

	if err := cmd.Flags().Parse(c); err != nil {
		return err
	}

	provider, err := cmd.makeProviderOrDie(ctx, apiCl)
	if err != nil {
		return err
	}

	// TODO when implementing the custom metrics provider, add cmd.WithCustomMetrics(provider) here
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

func (a *DatadogMetricsAdapter) makeProviderOrDie(ctx context.Context, apiCl *as.APIClient) (provider.ExternalMetricsProvider, error) {
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

	if config.Datadog.GetBool("external_metrics_provider.use_datadogmetric_crd") {
		return externalmetrics.NewDatadogMetricProvider(ctx, apiCl)
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
	if a.FlagSet.Lookup("cert-dir").Changed == false {
		// Ensure backward compatibility. Was hardcoded before.
		// Config flag is now to be added to the map `external_metrics_provider.config` as, `cert-dir`.
		a.SecureServing.ServerCert.CertDirectory = "/etc/datadog-agent/certificates"
	}
	if a.FlagSet.Lookup("secure-port").Changed == false {
		// Ensure backward compatibility. 443 by default, but will error out if incorrectly set.
		// refer to apiserver code in k8s.io/apiserver/pkg/server/option/serving.go
		a.SecureServing.BindPort = config.Datadog.GetInt("external_metrics_provider.port")
		// Default in External Metrics is TLS 1.2
		if !config.Datadog.GetBool("cluster_agent.allow_legacy_tls") {
			a.SecureServing.MinTLSVersion = tlsVersion13Str
		}
	}
	if err := a.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		log.Errorf("Failed to create self signed AuthN/Z configuration %#v", err)
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}
	return a.CustomMetricsAdapterServerOptions.Config()
}

// clearServerResources closes the connection and the server
// stops listening to new commands.
func clearServerResources() {
	if stopCh != nil {
		close(stopCh)
	}
}
